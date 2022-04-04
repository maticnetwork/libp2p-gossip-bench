package main

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"go.uber.org/zap"
)

const (
	AgentsNumber        = 400
	StartingPort        = 10000
	MaxPeers            = 10
	MsgSize             = 4096
	IpString            = "127.0.0.1"
	outputFileDirectory = "/tmp"
)

func main() {
	rand.Seed(time.Now().Unix())
	// remove file if exists
	// logger configuration
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{filepath.Join(outputFileDirectory, fmt.Sprintf("agents_%s.log", time.Now().Format(time.RFC3339)))}
	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	// flush buffer
	defer logger.Sync()

	connManager := network.NewConnManagerNetPipe()
	latencyData := lat.ReadLatencyDataFromJson()
	cluster := agent.NewCluster(logger, latencyData, agent.ClusterConfig{
		Ip:           IpString,
		StartingPort: StartingPort,
		MaxPeers:     MaxPeers,
		MsgSize:      MsgSize,
	})
	transportManager := network.NewTransportManager(connManager, cluster)

	fmt.Println("Start adding agents: ", AgentsNumber)

	// configure agents
	acfg := agent.NewDefaultAgentConfig()
	acfg.Transport = transportManager.Transport()
	// start agents in cluster
	agentsAdded, timeAdded := cluster.StartAgents(AgentsNumber, *acfg)
	fmt.Printf("Added %d agents. Ellapsed: %v\n", agentsAdded, timeAdded)
	// create cluster topology
	cluster.ConnectAgents(agent.LinearTopology{})

	fmt.Println("Gossip started")
	msgsPublishedCnt, msgsFailedCnt := cluster.GossipLoop(context.Background(), time.Millisecond*900, time.Second*30)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
}
