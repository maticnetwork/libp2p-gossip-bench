package network

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	ma "github.com/multiformats/go-multiaddr"
)

type ClusterAgent interface {
	Listen(ipString string, port int) error
	Connect(ClusterAgent) error
	Disconnect(ClusterAgent) error
	Gossip(data []byte) error
	Stop() error
	City() string
	NumPeers() int
	Addr() ma.Multiaddr
}

type MsgReceived func(lid, rid string)

type Cluster struct {
	lock sync.RWMutex

	latencyData *lat.LatencyData
	logger      *log.Logger
	agents      map[int]ClusterAgent // port to ClusterAgent
	port        int
	config      ClusterConfig
	agentStats  map[string]map[string]int64
}

type ClusterConfig struct {
	StartingPort int
	MaxPeers     int
	Ip           string
	MsgSize      int
}

var _ LatencyConnFactory = &Cluster{}

func NewCluster(logger *log.Logger, latencyData *lat.LatencyData, config ClusterConfig) *Cluster {
	return &Cluster{
		lock:        sync.RWMutex{},
		agents:      make(map[int]ClusterAgent),
		latencyData: latencyData,
		logger:      logger,
		config:      config,
		port:        config.StartingPort,
		agentStats:  make(map[string]map[string]int64),
	}
}

func (c *Cluster) AddAgent(agent ClusterAgent) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// trying to start agent on next available port...
	err := agent.Listen(c.config.Ip, c.port+1)
	if err != nil {
		return 0, fmt.Errorf("can not start agent for port %d, err: %v", c.port+1, err)
	}

	//... if agent is sucessfully started increment port and update map
	c.port++
	c.agents[c.port] = agent
	return c.port, nil
}

func (c *Cluster) GetAgent(id int) ClusterAgent {
	c.lock.RLock()
	defer c.lock.RUnlock()
	agent, exists := c.agents[id]
	if !exists {
		panic(fmt.Sprintf("Agent does not exist: %d", id))
	}
	return agent
}

func (c *Cluster) RemoveAgent(id int) error {
	agent := c.GetAgent(id)
	c.lock.Lock()
	defer c.lock.Unlock()
	err := agent.Stop()
	if err != nil {
		return err
	}
	delete(c.agents, id)
	return nil
}

func (c *Cluster) CreateConn(baseConn net.Conn, laddr, raddr ma.Multiaddr) (net.Conn, error) {
	laddrPort, err := laddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return nil, err
	}
	raddrPort, err := raddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		return nil, err
	}

	lport, _ := strconv.Atoi(laddrPort)
	rport, _ := strconv.Atoi(raddrPort)

	lagent, ragent := c.GetAgent(lport), c.GetAgent(rport)
	latencyDuration := c.latencyData.FindLatency(lagent.City(), ragent.City())

	nn := lat.Network{
		Kbps:    20 * 1024, // should I change this?
		Latency: latencyDuration,
		MTU:     1500,
	}
	return nn.Conn(baseConn)
}

func (c *Cluster) Gossip(from int, size int) {
	agent := c.GetAgent(from)
	buf := generateMsg(c.config.MsgSize)
	agent.Gossip(buf)
}

func (c *Cluster) Connect(fromID, toID int) error {
	from, to := c.GetAgent(fromID), c.GetAgent(toID)
	if from.NumPeers() == c.config.MaxPeers {
		return fmt.Errorf("agent %d has already maximum peers connected", fromID)
	}
	return from.Connect(to)
}

func (c *Cluster) Stop(portId int) error {
	return c.GetAgent(portId).Stop()
}

func (c *Cluster) Start(portId int) error {
	return c.GetAgent(portId).Listen(c.config.Ip, portId)
}

func (c *Cluster) StopAll() {
	for _, agnt := range c.agents {
		agnt.Stop()
	}
}

func (c *Cluster) GossipLoop(context context.Context, gossipTime time.Duration, timeout time.Duration) (int64, int64) {
	msgsPublishedCnt, msgsFailedCnt := int64(0), int64(0)
	ch := make(chan struct{})
	for _, agent := range c.agents {
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

		}(agent)
	}

	select {
	case <-time.After(timeout):
	case <-context.Done():
	}

	close(ch)
	return msgsPublishedCnt, msgsFailedCnt
}

func (c *Cluster) MsgReceived(lid, rid string) {
	if c.agentStats[lid] == nil {
		c.agentStats[lid] = make(map[string]int64)
	}
	c.agentStats[lid][rid]++
}

func (c *Cluster) PrintReceiversStats() {
	all := int64(0)
	byAgent := make(map[string]int64, 0)
	for k, v := range c.agentStats {
		cnt := int64(0)
		for _, num := range v {
			all += num
			cnt += num
		}
		byAgent[k] = cnt
	}

	fmt.Printf("Recieved messages: %d\n", all)
	fmt.Println("Recieved messages by agent")
	for k, v := range byAgent {
		fmt.Printf("Recieved messages by %s: %d\n", k, v)
	}
}

func generateMsg(size int) []byte {
	buf := make([]byte, size)
	rand.Read(buf)
	return buf
}
