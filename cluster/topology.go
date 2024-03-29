package cluster

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
)

type Topology interface {
	MakeConnections(agents map[int]agent.Agent)
}

type LinearTopology struct{}

func (t LinearTopology) MakeConnections(agents map[int]agent.Agent) {
	keys := make([]int, 0)
	for k := range agents {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	startTime := time.Now()
	connectionsNum := len(agents) - 1
	wg := sync.WaitGroup{}
	wg.Add(connectionsNum)
	cntAgentsConnected := int64(0)

	for i := 0; i <= connectionsNum-1; i++ {
		go func(i int) {
			defer wg.Done()

			source := agents[keys[i]]
			sink := agents[keys[i+1]]
			err := source.Connect(sink)
			if err != nil {
				fmt.Println("Could not connect peers ", keys[i], " ", keys[i+1], " ", err)
			} else {
				atomic.AddInt64(&cntAgentsConnected, 1)
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("Connected %d agents. Ellapsed: %v\n", cntAgentsConnected, time.Since(startTime))
}
