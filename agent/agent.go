package agent

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	configLibp2p "github.com/libp2p/go-libp2p/config"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	ma "github.com/multiformats/go-multiaddr"
)

const ()

type Agent struct {
	Host   host.Host
	Logger *log.Logger
	Config *AgentConfig
	Topic  *pubsub.Topic
}

var _ network.ClusterAgent = &Agent{}

type AgentConfig struct {
	Transport     configLibp2p.TptC
	MsgReceivedFn network.MsgReceived

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

func NewAgent(logger *log.Logger, config *AgentConfig) *Agent {
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

	readLoop(sub, host.ID(), a.Config.MsgReceivedFn)

	a.Host, a.Topic = host, topic
	return nil
}

func (a *Agent) Connect(remote network.ClusterAgent) error {
	remoteAddr := remote.(*Agent).Addr()
	peer, err := peer.AddrInfoFromP2pAddr(remoteAddr)
	if err != nil {
		return err
	}
	return a.Host.Connect(context.Background(), *peer)
}

func (a *Agent) Disconnect(remote network.ClusterAgent) error {
	remoteAddr := remote.(*Agent).Addr()
	for _, conn := range a.Host.Network().Conns() {
		if conn.RemoteMultiaddr().Equal(remoteAddr) {
			return conn.Close()
		}
	}
	return fmt.Errorf("could not disconnect from %s to %s", a.Host.Addrs()[0], remote)
}

func (a *Agent) Gossip(data []byte) error {
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

func readLoop(sub *pubsub.Subscription, lid peer.ID, handler func(lid, rid string, data []byte)) {
	go func() {
		for {
			raw, err := sub.Next(context.Background())
			if err != nil {
				fmt.Printf("Peer %v error receiving message on topic: %v\n", lid, err)
				continue
			}
			// fmt.Printf("Peer %v received data from %v\n", a.Host.ID(), from)
			handler(lid.Pretty(), raw.ReceivedFrom.Pretty(), raw.Data)
		}
	}()
}
