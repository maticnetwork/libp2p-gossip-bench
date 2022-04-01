package network

import (
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/utils"
)

// Holds list of all connection pairs (source and destination ClusterAgent)
type connectionsList []struct {
	src  ClusterAgent
	dest ClusterAgente
}

// Creates new connectionsList
func NewConnectionsList() connectionsList {
	return make(connectionsList, 0)
}

// Add new connection pair to the list
func (cl *connectionsList) Add(src, dest ClusterAgent) {
	*cl = append(*cl, struct {
		src  ClusterAgent
		dest ClusterAgent
	}{src: src, dest: dest})
}

// Connects all connection pairs (added via Add func)
// Execute Connect operations in multiple routines because of speed
func (cl connectionsList) ConnectAll() (int, int, time.Duration) {
	return utils.MultiRoutineRunner(len(cl), func(index int) error {
		ch := cl[index]
		return ch.src.Connect(ch.dest)
	})
}
