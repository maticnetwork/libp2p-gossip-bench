package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
)

const AgentsNumber = 400
const StartingPort = 10000
const MaxPeers = 10
const MsgSize = 4096
const IpString = "127.0.0.1"

var logger *log.Logger = log.New(os.Stdout, "Mesh: ", log.Flags())

func main() {
	rand.Seed(time.Now().Unix())

	connManager := network.NewConnManagerNetPipe()
	latencyData := lat.ReadLatencyDataFromJson()
	cluster := network.NewCluster(logger, latencyData.FindLatency, network.ClusterConfig{
		Ip:           IpString,
		StartingPort: StartingPort,
		MaxPeers:     MaxPeers,
		MsgSize:      MsgSize,
	})
	transportManager := network.NewTransportManager(connManager, cluster)

	fmt.Println("Start adding agents: ", AgentsNumber)

	agentsAdded, timeAdded := cluster.StartAgents(AgentsNumber, func(id int) (network.ClusterAgent, string) {
		ac := agent.NewDefaultAgentConfig()
		ac.Transport = transportManager.Transport()
		ac.MsgReceivedFn = cluster.MsgReceived
		return agent.NewAgent(logger, ac), latencyData.GetRandomCity()
	})
	fmt.Printf("Added %d agents. Ellapsed: %v\n", agentsAdded, timeAdded)
	cluster.ConnectAgents(network.LinearTopology{NumNodes: 10})

	fmt.Println("Gossip started")
	msgsPublishedCnt, msgsFailedCnt := cluster.GossipLoop(context.Background(), time.Millisecond*900, time.Second*30)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
	cluster.PrintReceiversStats()
}
