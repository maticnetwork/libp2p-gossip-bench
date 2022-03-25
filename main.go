package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
)

const AgentsNumber = 400
const StartAgentsRoutines = 100
const StartingPort = 10000
const MaxPeers = 10
const MsgSize = 4096
const IpString = "127.0.0.1"

var logger *log.Logger = log.New(os.Stdout, "Mesh: ", log.Flags())

func main() {
	rand.Seed(time.Now().Unix())

	connManager := network.NewConnManagerNetPipe()
	latencyData := lat.ReadLatencyData()
	cluster := network.NewCluster(logger, latencyData.FindLatency, network.ClusterConfig{
		Ip:           IpString,
		StartingPort: StartingPort,
		MaxPeers:     MaxPeers,
		MsgSize:      MsgSize,
	})
	transportManager := network.NewTransportManager(connManager, cluster)

	fmt.Println("Start adding agents: ", AgentsNumber)

	agentsAdded, timeAdded := cluster.StartAgents(AgentsNumber, StartAgentsRoutines, func(id int) (network.ClusterAgent, string) {
		ac := &agent.AgentConfig{
			Transport:     transportManager.Transport(),
			MsgReceivedFn: cluster.MsgReceived,
		}
		return agent.NewAgent(logger, ac), latencyData.GetRandomCity()
	})
	fmt.Printf("Added %d agents. Ellapsed: %v\n", agentsAdded, timeAdded)
	connectAgents(cluster)

	fmt.Println("Gossip started")
	msgsPublishedCnt, msgsFailedCnt := cluster.GossipLoop(context.Background(), time.Millisecond*900, time.Second*30)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
	cluster.PrintReceiversStats()
}

func connectAgents(cluster *network.Cluster) {
	startTime := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(AgentsNumber)
	cntAgentsConnected := int64(0)

	for i := 1; i <= AgentsNumber; i++ {
		go func(i int) {
			size := AgentsNumber / MaxPeers
			cnt := 0
			for j := i + size; j <= AgentsNumber; j += size {
				err := cluster.Connect(i+StartingPort, StartingPort+j)
				if err != nil {
					fmt.Println("Could not connect peers ", i+StartingPort, " ", StartingPort+j, " ", err)
				} else {
					cnt++
					atomic.AddInt64(&cntAgentsConnected, 1)
				}
			}
			wg.Done()
			if cnt > 0 {
				fmt.Printf("Peer %d dialed %d peers\n", i, cnt)
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("Connected %d agents. Ellapsed: %v\n", cntAgentsConnected, time.Since(startTime))
}
