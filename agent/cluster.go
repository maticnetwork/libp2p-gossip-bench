package agent

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"go.uber.org/zap"
)

type ClusterAgent interface {
	Listen(ipString string, port int) error
	Connect(ClusterAgent) error
	Disconnect(ClusterAgent) error
	Gossip(data []byte) error
	Stop() error
	NumPeers() int
}

type agentContainer struct {
	agent       ClusterAgent
	city        string
	port        int
	isValidator bool
}

type Cluster struct {
	lock sync.RWMutex

	latency *lat.LatencyData
	logger  *zap.Logger
	agents  map[int]agentContainer // port to ClusterAgent
	port    int
	config  ClusterConfig
}

type ClusterConfig struct {
	StartingPort int
	Ip           string
	MsgSize      int
	Kbps         int
	MTU          int
}

const defaultKbps = 20 * 1024
const defaultMTU = 1500

var _ network.LatencyConnFactory = &Cluster{}

func NewCluster(logger *zap.Logger, latency *lat.LatencyData, config ClusterConfig) *Cluster {
	return &Cluster{
		lock:    sync.RWMutex{},
		agents:  make(map[int]agentContainer),
		latency: latency,
		logger:  logger,
		config:  config,
		port:    config.StartingPort,
	}
}

func (c *Cluster) AddAgent(agent ClusterAgent, city string, isValidator bool) (int, error) {
	// we do not want to execute whole agent.listen in lock, thats is why we have locks at two places
	c.lock.Lock()
	c.port++
	listenPort := c.port
	c.lock.Unlock()

	// trying to start agent on next available port...
	err := agent.Listen(c.config.Ip, listenPort)
	if err != nil {
		return 0, fmt.Errorf("can not start agent for port %d, err: %v", listenPort, err)
	}

	//... if agent is sucessfully started update map
	c.lock.Lock()
	defer c.lock.Unlock()
	c.agents[listenPort] = agentContainer{agent: agent, city: city, port: listenPort, isValidator: isValidator}
	return listenPort, nil
}

func (c *Cluster) GetAgent(id int) ClusterAgent {
	return c.GetAgentContainer(id).agent
}

func (c *Cluster) GetAgentCity(id int) string {
	return c.GetAgentContainer(id).city
}

func (c *Cluster) GetAgentContainer(id int) agentContainer {
	c.lock.RLock()
	defer c.lock.RUnlock()
	agentCont, exists := c.agents[id]
	if !exists {
		panic(fmt.Sprintf("Agent does not exist: %d", id))
	}
	return agentCont
}

func (c *Cluster) RemoveAgent(id int) error {
	agent := c.GetAgent(id)
	err := agent.Stop()
	if err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.agents, id)
	return nil
}

func (c *Cluster) CreateConn(baseConn net.Conn, leftPortID, rightPortID int) (net.Conn, error) {
	lcity, rcity := c.GetAgentCity(leftPortID), c.GetAgentCity(rightPortID)
	latencyDuration := c.latency.Find(lcity, rcity)

	nn := lat.Network{
		Kbps:    getValue(c.config.Kbps, defaultKbps),
		Latency: latencyDuration,
		MTU:     getValue(c.config.MTU, defaultMTU),
	}
	return nn.Conn(baseConn)
}

func (c *Cluster) Gossip(fromID int, size int) {
	agent := c.GetAgent(fromID)
	buf := generateMsg(c.config.MsgSize)
	agent.Gossip(buf)
}

func (c *Cluster) Stop(portID int) error {
	return c.GetAgent(portID).Stop()
}

func (c *Cluster) Start(portID int) error {
	return c.GetAgent(portID).Listen(c.config.Ip, portID)
}

func (c *Cluster) StopAll() {
	for _, cont := range c.agents {
		cont.agent.Stop()
	}
}

func (c *Cluster) GossipLoop(context context.Context, gossipTime time.Duration, timeout time.Duration) (int64, int64) {
	msgsPublishedCnt, msgsFailedCnt := int64(0), int64(0)
	ch := make(chan struct{})
	for _, cont := range c.agents {
		if !cont.isValidator {
			continue
		}

		go func(a ClusterAgent) {
			tm := time.NewTicker(gossipTime)
			defer tm.Stop()
		outer:
			for {
				select {
				case <-tm.C:
					err := a.Gossip(generateMsg(c.config.MsgSize))
					if err == nil {
						atomic.AddInt64(&msgsPublishedCnt, 1)
					} else {
						atomic.AddInt64(&msgsFailedCnt, 1)
					}
				case <-ch:
					break outer
				}
			}

		}(cont.agent)
	}

	select {
	case <-time.After(timeout):
	case <-context.Done():
	}

	close(ch)
	return msgsPublishedCnt, msgsFailedCnt
}

func (c *Cluster) ConnectAgents(topology Topology) {
	topology.MakeConnections(c.agents)
}

func generateMsg(size int) []byte {
	buf := make([]byte, size)
	rand.Read(buf)
	return buf
}

func getValue(value, def int) int {
	if value != 0 {
		return value
	}
	return def
}
