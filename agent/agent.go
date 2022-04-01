package agent

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	libp2pnetwork "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	configLibp2p "github.com/libp2p/go-libp2p/config"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/zap"
)

var connectionHelper agentConnectionHelper = newAgentConnectionHelper()

type Agent struct {
	Host   host.Host
	Logger *zap.Logger
	Config *AgentConfig
	Topic  *pubsub.Topic
}

var _ ClusterAgent = &Agent{}

type AgentConfig struct {
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

func NewDefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		GossipSubD:                 8,
		GossipSubDlo:               6,
		GossipSubDhi:               12,
		GossipSubMcacheLen:         6,
		GossipSubMcacheGossip:      3,
		GossipSubSeenTTL:           550,
		GossipSubFanoutTTL:         60000000000,
		GossipSubHeartbeatInterval: 700 * time.Millisecond,
		PubsubQueueSize:            600,
	}
}

// gossipsub.
// topic for pubsub
const topicName = "Topic"

func NewAgent(logger *zap.Logger, config *AgentConfig) *Agent {
	return &Agent{
		Logger: logger,
		Config: config,
	}
}

func (a *Agent) Listen(ipString string, port int) error {
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
			a.Logger.Info("message received",
				zap.String("peer", host.ID().Pretty()),
				zap.String("from", raw.ReceivedFrom.Pretty()),
			)
		}
	}()

	a.Host, a.Topic = host, topic
	return nil
}

func (a *Agent) Connect(remote ClusterAgent) error {
	localAddr, remoteAddr := a.Host.Addrs()[0], remote.(*Agent).Addr()
	peer, err := peer.AddrInfoFromP2pAddr(remoteAddr)
	if err != nil {
		return err
	}

	// do not exit Connect until both nodes are connected
	connectionHelper.Add(localAddr, remoteAddr)
	defer connectionHelper.Delete(localAddr, remoteAddr)
	err = a.Host.Connect(context.Background(), *peer)
	if err != nil {
		return err
	}
	connectionHelper.Wait(localAddr, remoteAddr)
	return nil
}

func (a *Agent) Disconnect(remote ClusterAgent) error {
	remoteAddr := remote.(*Agent).Addr()
	for _, conn := range a.Host.Network().Conns() {
		if conn.RemoteMultiaddr().Equal(remoteAddr) {
			return conn.Close()
		}
	}
	return fmt.Errorf("could not disconnect from %s to %s", a.Host.Addrs()[0], remote)
}

func (a *Agent) Gossip(data []byte) error {
	a.Logger.Info("message sent",
		zap.String("peer", a.Host.ID().Pretty()),
	)
	return a.Topic.Publish(context.Background(), data)
}

func (a *Agent) Stop() error {
	return a.Host.Close()
}

func (a *Agent) NumPeers() int {
	return a.Host.Peerstore().Peers().Len() - 1 // libp2p holds itself in list
}

func (a *Agent) Addr() ma.Multiaddr {
	port, _ := a.Host.Addrs()[0].ValueForProtocol(ma.P_TCP)
	ip, _ := a.Host.Addrs()[0].ValueForProtocol(ma.P_IP4)
	id := fmt.Sprintf("/ip4/%s/tcp/%s/p2p/%s", ip, port, a.Host.ID())
	listenAddr, _ := ma.NewMultiaddr(id)
	return listenAddr
}

// creates a custom gossipsub parameter set.
func (a *Agent) getPubsubGossipParams() pubsub.GossipSubParams {
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
