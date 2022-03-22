package network

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
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

type Cluster struct {
	lock sync.RWMutex

	latencyData *lat.LatencyData
	logger      *log.Logger
	agents      map[int]ClusterAgent // port to ClusterAgent
	port        int
	maxPeers    int
	ipString    string
}

var _ LatencyConnFactory = &Cluster{}

func NewCluster(logger *log.Logger, latencyData *lat.LatencyData, ipString string, startingPort int, maxPeers int) *Cluster {
	return &Cluster{
		lock:        sync.RWMutex{},
		agents:      make(map[int]ClusterAgent),
		latencyData: latencyData,
		logger:      logger,
		port:        startingPort,
		maxPeers:    maxPeers,
		ipString:    ipString,
	}
}

func (c *Cluster) AddAgent(agent ClusterAgent) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// trying to start agent on next available port...
	err := agent.Listen(c.ipString, c.port+1)
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

func (m *Cluster) CreateConn(baseConn net.Conn, laddr, raddr ma.Multiaddr) (net.Conn, error) {
	laddrPort, err := laddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		panic(err)
	}
	raddrPort, err := raddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		panic(err)
	}

	lport, _ := strconv.Atoi(laddrPort)
	rport, _ := strconv.Atoi(raddrPort)

	lagent, ragent := m.GetAgent(lport), m.GetAgent(rport)
	latencyDuration := m.latencyData.FindLatency(lagent.City(), ragent.City())

	nn := lat.Network{
		Kbps:    20 * 1024, // should I change this?
		Latency: latencyDuration,
		MTU:     1500,
	}
	return nn.Conn(baseConn)
}

func (m *Cluster) Gossip(from int, size int) {
	agent := m.GetAgent(from)
	buf := generateMsg(size, agent.Addr())
	agent.Gossip(buf)
}

func (m *Cluster) Connect(fromID, toID int) error {
	from, to := m.GetAgent(fromID), m.GetAgent(toID)
	if from.NumPeers() == m.maxPeers {
		return fmt.Errorf("agent %d has already maximum peers connected", fromID)
	}
	return from.Connect(to)
}

func (m *Cluster) Stop(portId int) error {
	return m.GetAgent(portId).Stop()
}

func (m *Cluster) Start(portId int) error {
	return m.GetAgent(portId).Listen(m.ipString, portId)
}

func (m *Cluster) StopAll() {
	for _, agnt := range m.agents {
		agnt.Stop()
	}
}

func (m *Cluster) GossipLoop(context context.Context, gossipTime time.Duration, timeout time.Duration) {
	ch := make(chan struct{})
	for _, agent := range m.agents {
		go func(a ClusterAgent) {
			tm := time.NewTicker(gossipTime)
			defer tm.Stop()
		outer:
			for {
				select {
				case <-tm.C:
					a.Gossip(generateMsg(10, a.Addr()))
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
}

func generateMsg(size int, addr ma.Multiaddr) []byte {
	buf := make([]byte, size)
	rand.Read(buf)
	p, _ := addr.ValueForProtocol(ma.P_TCP)
	buf[0] = byte(p[0])
	buf[1] = byte(p[1])
	return buf
}
