package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
)

func main() {
	rand.Seed(time.Now().Unix())

	size := 200
	gossipSize := 100
	msgSize := 1024
	maxPeers := 10

	log := log.New(os.Stdout, "Mesh: ", log.Flags())

	m := &latency.Mesh{
		Latency:  latency.ReadLatencyData(),
		Logger:   log,
		Port:     30000,
		MaxPeers: maxPeers,
	}
	m.Manager = &network.Manager{}

	for i := 0; i < size; i++ {
		m.RunServer("srv_" + strconv.Itoa(i))
	}

	// join them in a line
	var wg sync.WaitGroup
	for i := 0; i < size-1; i++ {
		wg.Add(1)

		go func(i int) {
			m.Join("srv_"+strconv.Itoa(i), "srv_"+strconv.Itoa(i+1))
			wg.Done()
		}(i)
	}
	wg.Wait()

	time.Sleep(1 * time.Minute)

	// m.waitForPeers(3)

	for i := 0; i < gossipSize; i++ {
		m.Gossip("srv_"+strconv.Itoa(i), msgSize)
	}
	for i := 0; i < gossipSize; i++ {
		m.Gossip("srv_"+strconv.Itoa(i), msgSize)
	}

	time.Sleep(1 * time.Second)

	compute := func(show bool) {
		total := 0
		for _, p := range m.Agents {
			if show {
				fmt.Println(p.Config.City)
			}
		}
		fmt.Println(total / len(m.Agents))
	}

	compute(false)

	time.Sleep(1 * time.Second)

	compute(false)

	time.Sleep(1 * time.Second)

	compute(false)

	// 50 nodos, full, 1 second 95%
	//
	// handleSignals()
}

func handleSignals() {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-signalCh
	os.Exit(0)
}
