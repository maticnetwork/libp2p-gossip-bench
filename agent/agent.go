package agent

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	libp2pnetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	configLibp2p "github.com/libp2p/go-libp2p/config"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

var byteOrder = binary.BigEndian

var connectionHelper agentConnectionHelper = newAgentConnectionHelper()

type Agent interface {
	Listen(ipString string, port int) error
	Connect(Agent) error
	Disconnect(Agent) error
	SendMessage(size int, isUseful bool) error
	Stop() error
	NumPeers() int
	GetCity() string
	GetPort() int
	IsValidator() bool
}

type AgentConfig interface {
	SetDefaults()
}

func NewAgent(logger *zap.Logger, port int, city string, isValidator bool, config AgentConfig) Agent {
	switch v := config.(type) {
	case *GossipConfig:
		return &GossipAgent{
			Logger:    logger,
			Config:    v,
			City:      city,
			Port:      port,
			Validator: isValidator,
		}
	default:
		panic("I don't know that Agent Config")
	}
}

type GossipAgent struct {
	City      string
	Port      int
	Validator bool
	Host      host.Host
	Logger    *zap.Logger
	Config    *GossipConfig
	Topic     *pubsub.Topic
}

type GossipConfig struct {
	Transport configLibp2p.TptC

	// overlay parameters
	GossipSubD   int // topic stable mesh target count
	GossipSubDlo int // topic stable mesh low watermark
	GossipSubDhi int // topic stable mesh high watermark

	// gossip parameters
	GossipSubMcacheLen    int // number of windows to retain full messages in cache for `IWANT` responses
	GossipSubMcacheGossip int // number of windows to gossip about
	GossipSubSeenTTL      int // number of heartbeat intervals to retain message IDs

	// fanout ttl
	GossipSubFanoutTTL int // TTL for fanout maps for topics we are not subscribed to but have published to, in nano seconds

	// heartbeat interval
	GossipSubHeartbeatInterval time.Duration // frequency of heartbeat, milliseconds

	// pubsubQueueSize is the size that we assign to our validation queue and outbound message queue for
	PubsubQueueSize int
}

func (c *GossipConfig) SetDefaults() {
	c.GossipSubD = 8
	c.GossipSubDlo = 6
	c.GossipSubDhi = 12
	c.GossipSubMcacheLen = 6
	c.GossipSubMcacheGossip = 3
	c.GossipSubSeenTTL = 550
	c.GossipSubFanoutTTL = 60000000000
	c.GossipSubHeartbeatInterval = 700 * time.Millisecond
	c.PubsubQueueSize = 600
}

// gossipsub.
// topic for pubsub
const topicName = "Topic"

type packet struct {
	MessageID uuid.UUID
	IsUseful  bool
}

func (a *GossipAgent) Listen(ipString string, port int) error {
	listenAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", ipString, port))
	if err != nil {
		return err
	}

	host, err := libp2p.New(
		libp2p.ListenAddrs(listenAddr),
		libp2p.Transport(a.Config.Transport),
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	host.Network().Notify(&libp2pnetwork.NotifyBundle{
		ConnectedF: func(n libp2pnetwork.Network, conn libp2pnetwork.Conn) {
			// notify connectionHelper that peer with conn.RemoteMultiaddr() is connected to peer with conn.LocalMultiaddr()
			connectionHelper.Connected(conn.LocalMultiaddr(), conn.RemoteMultiaddr())
		},
	})

	// start gossip protocol
	psOpts := []pubsub.Option{
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithPeerOutboundQueueSize(a.Config.PubsubQueueSize),
		pubsub.WithValidateQueueSize(a.Config.PubsubQueueSize),
		pubsub.WithGossipSubParams(a.getPubsubGossipParams()),
	}
	ps, err := pubsub.NewGossipSub(context.Background(), host, psOpts...)
	if err != nil {
		return err
	}

	// topic
	topic, err := ps.Join(topicName)
	if err != nil {
		return err
	}
	sub, err := topic.Subscribe()
	if err != nil {
		return err
	}
	a.Logger.Info("agent successfully subscribed", zap.String("topic", topicName))

	// read messages and print stats to logger
	go func() {
		for {
			raw, err := sub.Next(context.Background())
			if err != nil {
				fmt.Printf("Peer %v error receiving message on topic: %v\n", host.ID(), err)
				continue
			}

			// read message data from received bytes
			var data packet
			if err := readMessage(raw.Data, &data); err != nil {
				fmt.Printf("Peer %v error reading message ID: %v\n", host.ID(), err)

			}
			a.Logger.Info("message received",
				zap.String("peer", host.ID().Pretty()),
				zap.String("direction", "received"),
				zap.String("msgID", data.MessageID.String()),
				zap.Bool("aggregateStats", data.IsUseful),
				zap.String("from", raw.ReceivedFrom.Pretty()),
			)
		}
	}()

	a.Host, a.Topic = host, topic
	return nil
}

func (a *GossipAgent) Connect(remote Agent) error {
	localAddr, remoteAddr := a.Host.Addrs()[0], remote.(*GossipAgent).Addr()
	peer, err := peer.AddrInfoFromP2pAddr(remoteAddr)
	if err != nil {
		return err
	}

	// do not exit Connect until both nodes are connected
	if err := connectionHelper.Add(localAddr, remoteAddr); err != nil {
		return err
	}
	defer connectionHelper.Delete(localAddr, remoteAddr)
	err = a.Host.Connect(context.Background(), *peer)
	if err != nil {
		return err
	}
	connectionHelper.Wait(localAddr, remoteAddr)
	return nil
}

func (a *GossipAgent) Disconnect(remote Agent) error {
	remoteAddr := remote.(*GossipAgent).Addr()
	for _, conn := range a.Host.Network().Conns() {
		if conn.RemoteMultiaddr().Equal(remoteAddr) {
			return conn.Close()
		}
	}
	return fmt.Errorf("could not disconnect from %s to %s", a.Host.Addrs()[0], remote)
}

func (a *GossipAgent) SendMessage(size int, isUseful bool) error {
	data := packet{
		MessageID: uuid.New(),
		IsUseful:  isUseful,
	}
	msg, err := createMessage(data, size)
	if err != nil {
		return err
	}

	// log that message was sent so we can aggregate stats later
	a.Logger.Info("message sent",
		zap.String("peer", a.Host.ID().Pretty()),
		zap.String("direction", "sent"),
		zap.Bool("aggregateStats", isUseful),
		zap.String("msgID", data.MessageID.String()),
	)

	return a.Topic.Publish(context.Background(), msg)
}

func (a *GossipAgent) Stop() error {
	return a.Host.Close()
}

func (a *GossipAgent) NumPeers() int {
	return a.Host.Peerstore().Peers().Len() - 1 // libp2p holds itself in list
}

func (a *GossipAgent) Addr() ma.Multiaddr {
	port, _ := a.Host.Addrs()[0].ValueForProtocol(ma.P_TCP)
	ip, _ := a.Host.Addrs()[0].ValueForProtocol(ma.P_IP4)
	id := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, port, a.Host.ID())
	listenAddr, _ := ma.NewMultiaddr(id)
	return listenAddr
}

func (a *GossipAgent) GetCity() string {
	return a.City
}

func (a *GossipAgent) GetPort() int {
	return a.Port
}

func (a *GossipAgent) IsValidator() bool {
	return a.Validator
}

// creates a custom gossipsub parameter set.
func (a *GossipAgent) getPubsubGossipParams() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = a.Config.GossipSubDlo
	gParams.D = a.Config.GossipSubD
	gParams.HeartbeatInterval = a.Config.GossipSubHeartbeatInterval
	gParams.HistoryLength = a.Config.GossipSubMcacheLen
	gParams.HistoryGossip = a.Config.GossipSubMcacheGossip

	// Set a larger gossip history to ensure that slower
	// messages have a longer time to be propagated. This
	// comes with the tradeoff of larger memory usage and
	// size of the seen message cache.
	//if features.Get().EnableLargerGossipHistory {
	//	gParams.HistoryLength = 12
	//	gParams.HistoryGossip = 5
	//}
	return gParams
}

// createMessage function fills the necessarry message size with
// unique ID and the rest with random bytes
func createMessage(data packet, size int) ([]byte, error) {
	buf := new(bytes.Buffer)

	// write the ID in first
	if err := binary.Write(buf, byteOrder, data); err != nil {
		return nil, err
	}

	// to achieve configured message size,
	// fill the rest of the buffer with rand bytes
	if size < buf.Len() {
		return nil, fmt.Errorf("message size can not be less than %d", buf.Len())
	}
	filler := make([]byte, size-buf.Len())
	rand.Read(filler)

	// write filler bytes into message after message ID
	if err := binary.Write(buf, byteOrder, filler); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// readMessage is used to decode received messages into a passed data object
func readMessage(b []byte, data *packet) error {
	buf := bytes.NewBuffer(b)
	if err := binary.Read(buf, byteOrder, data); err != nil {
		return err
	}
	return nil
}
