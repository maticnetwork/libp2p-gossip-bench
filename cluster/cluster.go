package cluster

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/utils"
	"go.uber.org/zap"
)

type Cluster struct {
	lock sync.RWMutex

	latency      *lat.LatencyData
	logger       *zap.Logger
	agents       map[int]agent.Agent // agents mapped by their listening port
	startingPort int
	config       ClusterConfig
}

type ClusterConfig struct {
	ValidatorCount int // number of validator agents in a cluster
	StartingPort   int
	Ip             string
	MsgSize        int
	Kbps           int
	MTU            int
}

const defaultKbps = 20 * 1024
const defaultMTU = 1500

func NewCluster(logger *zap.Logger, latency *lat.LatencyData, c ClusterConfig) *Cluster {
	return &Cluster{
		lock:         sync.RWMutex{},
		agents:       make(map[int]agent.Agent),
		latency:      latency,
		logger:       logger,
		config:       c,
		startingPort: c.StartingPort,
	}
}

func (c *Cluster) AddAgents(config agent.AgentConfig, numOfAgents int) {
	for i := 1; i <= numOfAgents; i++ {
		city := c.latency.GetRandomCity()
		isValidator := i < c.config.ValidatorCount
		c.agents[i] = agent.NewAgent(c.logger, i, city, isValidator, config)
	}
}

func (c *Cluster) GetAgent(id int) agent.Agent {
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
	err := agent.Stop()
	if err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.agents, id)
	return nil
}

func (c *Cluster) GetAgentCity(id int) string {
	return c.GetAgent(id).GetCity()
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

func (c *Cluster) Stop(portID int) error {
	return c.GetAgent(portID).Stop()
}

func (c *Cluster) Start(portID int) error {
	return c.GetAgent(portID).Listen(c.config.Ip, portID)
}

func (c *Cluster) StopAll() {
	for _, agent := range c.agents {
		agent.Stop()
	}
}

func (c *Cluster) StartMessaging(ctx context.Context, messaging Messaging) (int64, int64) {
	// filter validators
	validators := filterValidators(c.agents)
	fmt.Println("Messaging started")
	publishedCnt, failedCnt := messaging.Loop(ctx, validators)
	fmt.Println("Messaging is complete")

	return publishedCnt, failedCnt
}

func (c *Cluster) StartAgents() (int, int, time.Duration) {
	const (
		maxRoutines     = 1000
		itemsPerRoutine = 1
	)

	agentsNumber := len(c.agents)
	started, failed, time := utils.MultiRoutineRunner(agentsNumber, itemsPerRoutine, maxRoutines, func(index int) error {
		agent := c.GetAgent(index)
		port := c.startingPort + index
		if err := agent.Listen(c.config.Ip, port); err != nil {
			c.RemoveAgent(index)
			return fmt.Errorf("can not start agent for port %d, err: %v", port, err)
		}
		return nil
	})

	return started, failed, time
}

func (c *Cluster) ConnectAgents(topology Topology) {
	topology.MakeConnections(c.agents)
}

func getValue(value, def int) int {
	if value != 0 {
		return value
	}
	return def
}

func filterValidators(agents map[int]agent.Agent) []agent.Agent {
	var result []agent.Agent
	for _, a := range agents {
		if a.IsValidator() {
			result = append(result, a)
		}
	}

	return result
}
