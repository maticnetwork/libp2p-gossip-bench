package agent

import (
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/utils"
)

const (
	itemsPerRoutine = 5
	maxRoutines     = 50
)

// Holds list of all connection pairs (source and destination ClusterAgent)
type connectionsList []struct {
	src  Agent
	dest Agent
}

// Add new connection pair to the list
func (cl *connectionsList) Add(src, dest Agent) {
	*cl = append(*cl, struct {
		src  Agent
		dest Agent
	}{src: src, dest: dest})
}

// Connects all connection pairs (added via Add func)
// Execute Connect operations in multiple routines because of speed
func (cl connectionsList) ConnectAll() (int, int, time.Duration) {
	return utils.MultiRoutineRunner(len(cl), itemsPerRoutine, maxRoutines, func(index int) error {
		ch := cl[index]
		return ch.src.Connect(ch.dest)
	})
}
