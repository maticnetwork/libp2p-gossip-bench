package network

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
)

type ClusterAgent interface {
	Listen(ipString string, port int) error
	Connect(ClusterAgent) error
	Disconnect(ClusterAgent) error
	Gossip(data []byte) error
	Stop() error
	NumPeers() int
}

type MsgReceived func(lid, rid string, data []byte)

type LatencyFinder func(lcity, rcity string) time.Duration

type agentContainer struct {
	agent ClusterAgent
	city  string
}

type Cluster struct {
	lock sync.RWMutex

	latencyFinder LatencyFinder
	logger        *log.Logger
	agents        map[int]agentContainer // port to ClusterAgent
	port          int
	config        ClusterConfig
	agentStats    map[string]map[string]int64
}

type ClusterConfig struct {
	StartingPort int
	MaxPeers     int
	Ip           string
	MsgSize      int
	Kbps         int
	MTU          int
}

const defaultKbps = 20 * 1024
const defaultMTU = 1500
const routinesNumber = 5

var _ LatencyConnFactory = &Cluster{}

func NewCluster(logger *log.Logger, latencyFinder LatencyFinder, config ClusterConfig) *Cluster {
	return &Cluster{
		lock:          sync.RWMutex{},
		agents:        make(map[int]agentContainer),
		latencyFinder: latencyFinder,
		logger:        logger,
		config:        config,
		port:          config.StartingPort,
		agentStats:    make(map[string]map[string]int64),
	}
}

func (c *Cluster) AddAgent(agent ClusterAgent, city string) (int, error) {
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
	c.agents[listenPort] = agentContainer{agent: agent, city: city}
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
	latencyDuration := c.latencyFinder(lcity, rcity)

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

func (c *Cluster) Connect(fromID, toID int) error {
	from, to := c.GetAgent(fromID), c.GetAgent(toID)
	if from.NumPeers() == c.config.MaxPeers {
		return fmt.Errorf("agent %d has already maximum peers connected", fromID)
	}
	return from.Connect(to)
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

func (c *Cluster) MsgReceived(lid, rid string, data []byte) {
	c.lock.Lock()
	defer c.lock.Unlock()
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

func (c *Cluster) StartAgents(agentsNumber int, factory func(id int) (ClusterAgent, string)) (int64, time.Duration) {
	routinesCount := routinesNumber
	if routinesCount > agentsNumber {
		routinesCount = agentsNumber
	}
	startTime := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(routinesCount)

	cntAgentsStarted := int64(0)
	cntPerRoutine := (agentsNumber + routinesCount - 1) / routinesCount
	for i := 0; i < routinesCount; i++ {
		cnt, offset := cntPerRoutine, i*cntPerRoutine
		if cnt+offset > agentsNumber {
			cnt = agentsNumber - offset
		}

		go func(offset, cntAgents int) {
			for i := 0; i < cntAgents; i++ {
				agent, city := factory(offset + i)
				_, err := c.AddAgent(agent, city)
				if err != nil {
					fmt.Printf("Could not start peer %d\n", atomic.LoadInt64(&cntAgentsStarted))
				} else {
					atomic.AddInt64(&cntAgentsStarted, 1)
				}
			}

			wg.Done()
		}(offset, cnt)
	}

	wg.Wait()
	return cntAgentsStarted, time.Since(startTime)
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
