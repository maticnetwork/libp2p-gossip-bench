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

const AgentsNumber = 100
const StartAgentsRoutines = 40
const StartingPort = 10000
const MaxPeers = 7
const MsgSize = 4096
const IpString = "127.0.0.1"

var logger *log.Logger = log.New(os.Stdout, "Mesh: ", log.Flags())

func main() {
	rand.Seed(time.Now().Unix())

	connManager := network.NewConnManagerNetPipe()
	latencyData := lat.ReadLatencyData()
	cluster := network.NewCluster(logger, latencyData, network.ClusterConfig{
		Ip:           IpString,
		StartingPort: StartingPort,
		MaxPeers:     MaxPeers,
		MsgSize:      MsgSize,
	})
	transportManager := network.NewTransportManager(connManager, cluster)

	fmt.Println("Start adding agents: ", AgentsNumber)

	startAgents(cluster, transportManager, latencyData)
	connectAgents(cluster)

	fmt.Println("Gossip started")
	msgsPublishedCnt, msgsFailedCnt := cluster.GossipLoop(context.Background(), time.Millisecond*900, time.Second*30)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
	cluster.PrintReceiversStats()
}

func startAgents(cluster *network.Cluster, transportManager *network.TransportManager, latencyData *lat.LatencyData) {
	startTime := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(StartAgentsRoutines)

	cntAgentsStarted := int64(0)
	cntPerRoutine := (AgentsNumber + StartAgentsRoutines - 1) / StartAgentsRoutines
	for i := 0; i < StartAgentsRoutines; i++ {
		cnt := cntPerRoutine
		if cnt+i*cntPerRoutine > AgentsNumber {
			cnt = AgentsNumber - i*cntPerRoutine
		}

		go func(cntAgents int) {
			for i := 1; i <= cntAgents; i++ {
				ac := &agent.AgentConfig{
					City:          latencyData.GetRandomCity(),
					Transport:     transportManager.Transport(),
					MsgReceivedFn: cluster.MsgReceived,
				}
				a := agent.NewAgent(logger, ac)
				_, err := cluster.AddAgent(a)
				if err != nil {
					fmt.Printf("Could not start peer %d\n", atomic.LoadInt64(&cntAgentsStarted))
				} else {
					atomic.AddInt64(&cntAgentsStarted, 1)
				}
			}

			wg.Done()
		}(cnt)
	}

	wg.Wait()
	fmt.Printf("Started %d agents. Ellapsed: %v\n", cntAgentsStarted, time.Since(startTime))
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
