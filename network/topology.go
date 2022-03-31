package network

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Topology interface {
	MakeConnections(agents map[int]agentContainer)
}

type LinearTopology struct{}

func (t LinearTopology) MakeConnections(agents map[int]agentContainer) {
	keys := make([]int, 0)
	for k := range agents {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	startTime := time.Now()
	wg := sync.WaitGroup{}
	wg.Add(len(agents))
	cntAgentsConnected := int64(0)

	for i, key := range keys {
		if i <= len(keys)-1 {
			go func(i int) {
				defer wg.Done()

				source := agents[key].agent
				sink := agents[keys[i+1]].agent
				err := source.Connect(sink)
				if err != nil {
					fmt.Println("Could not connect peers ", key, " ", keys[i+1], " ", err)
				} else {
					atomic.AddInt64(&cntAgentsConnected, 1)
				}
			}(i)
		}
	}

	wg.Wait()
	fmt.Printf("Connected %d agents. Ellapsed: %v\n", cntAgentsConnected, time.Since(startTime))
}
