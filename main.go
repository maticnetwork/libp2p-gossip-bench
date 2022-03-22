package main

import (
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
)

const AgentsNumber = 2
const MsgSize = 1024
const StartingPort = 10000
const MaxPeers = 2
const IpString = "127.0.0.1"

var logger *log.Logger = log.New(os.Stdout, "Mesh: ", log.Flags())

func main() {
	rand.Seed(time.Now().Unix())

	connManager := network.NewConnManagerNetPipeAsync()
	latencyData := lat.ReadLatencyData()
	cluster := network.NewCluster(logger, latencyData, IpString, StartingPort, MaxPeers)
	transportManager := network.NewTransportManager(connManager, cluster)

	for i := 0; i < AgentsNumber; i++ {
		ac := &agent.AgentConfig{
			City:      latencyData.GetRandomCity(),
			Transport: transportManager.Transport(),
		}
		a := agent.NewAgent(logger, ac)
		_, err := cluster.AddAgent(a)
		if err != nil {
			panic(err)
		}
	}

	err := cluster.Connect(1+StartingPort, 2+StartingPort)
	if err != nil {
		panic(err)
	}

	// for i := 1; i <= AgentsNumber; i++ {
	// 	size := AgentsNumber / MaxPeers
	// 	for j := i + size; j < AgentsNumber; j += size {
	// 		err := cluster.Connect(i+StartingPort, StartingPort+j)
	// 		if err != nil {
	// 			panic(err)
	// 		}
	// 	}
	// }

	// cluster.GossipLoop(context.Background(), time.Second*40, time.Millisecond*900)
}
