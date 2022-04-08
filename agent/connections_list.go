package agent

import (
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/utils"
)

const (
	itemsPerRoutine = 50
	maxRoutines     = 50
)

// Holds list of all connection pairs (source and destination ClusterAgent)
type connectionsList []struct {
	src  Agent
	dest Agent
}

// Creates new connectionsList
func NewConnectionsList() connectionsList {
	return make(connectionsList, 0)
}

// Add new connection pair to the list
func (cl *connectionsList) Add(src, dest Agent) {
	*cl = append(*cl, struct {
		src  Agent
		dest Agent
	}{src: src, dest: dest})
}

// Add connections between nearby (by index in slice) agents
func (cl *connectionsList) AddNearbyConnections(agents []agentContainer, peering uint) {
	connectionsCount := make([]uint, len(agents))
	for i := 0; i < len(agents)-1; i++ {
		for j := i + 1; j < len(agents) && connectionsCount[i] < peering; j++ {
			cl.Add(agents[i].agent, agents[j].agent)
			connectionsCount[i]++
			connectionsCount[j]++
		}
	}
}

// Connects all connection pairs (added via Add func)
// Execute Connect operations in multiple routines because of speed
func (cl connectionsList) ConnectAll() (int, int, time.Duration) {
	return utils.MultiRoutineRunner(len(cl), itemsPerRoutine, maxRoutines, func(index int) error {
		ch := cl[index]
		return ch.src.Connect(ch.dest)
	})
}
