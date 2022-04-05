package agent

import (
	"fmt"
	"math/rand"
)

type RandomTopology struct {
	MaxPeers  int  // maximum number of connected peers
	Count     int  // count of connections in topology
	Connected bool // if true there will always be linear connection beetwen nodes
}

// Creates random topology connections between peers
func (t RandomTopology) MakeConnections(agents map[int]agentContainer) {
	count := t.Count
	connections := NewConnectionsList()

	possiblePeers := make([]agentContainer, len(agents))         // list of all possible peers that can be choosen as source in one connection
	peerMap := make(map[int]*rndToplogyPeer, len(possiblePeers)) // map portID -> *rndToplogyPeer

	// initializing structures
	index := 0
	for _, agentCont := range agents {
		possiblePeers[index] = agentCont
		index++
	}

	for portID, agentCont := range agents {
		peerMap[portID] = newRndTopologyPeer(portID, agentCont.agent, agents)
	}

	// create linear connections between nodes if needed
	if t.Connected {
		for i := 0; i <= len(possiblePeers)-2; i++ {
			src, dst := possiblePeers[i], possiblePeers[i+1]
			peerMap[src.port].RemovePortId(dst.port)
			peerMap[src.port].IncConnCount()
			peerMap[dst.port].RemovePortId(src.port)
			peerMap[dst.port].IncConnCount()
			connections.Add(src.agent, dst.agent)
			count--
		}
	}

	// execute actual random population
	for count > 0 && len(possiblePeers) > 0 {
		srcPeerIndex := rand.Intn(len(possiblePeers))

		srcAgentCont := possiblePeers[srcPeerIndex]
		srcPortID := srcAgentCont.port
		srcPeer := peerMap[srcPortID]

		dstPortID := srcPeer.PopRandomPortID()
		dstPeer, dstPeerExists := peerMap[dstPortID]

		// it is possible that dest peer is removed but this is not updated yet on src peer
		if !dstPeerExists || !dstPeer.CanBeUsed(t.MaxPeers) {
			if !srcPeer.CanBeUsed(t.MaxPeers) {
				delete(peerMap, srcPortID)
				possiblePeers[srcPeerIndex] = possiblePeers[len(possiblePeers)-1]
				possiblePeers = possiblePeers[:len(possiblePeers)-1]
			}
			continue
		}

		// remove src port id from dst peer
		dstPeer.RemovePortId(srcPortID)
		count--

		// increment number of connections for both peers
		srcPeer.IncConnCount()
		dstPeer.IncConnCount()

		connections.Add(srcPeer.agent, dstPeer.agent) // add agent pair to connections list

		// if some of src or dest peers can not be used any more
		// (has reached maximal connections count or all possible connections are used)
		// we need to remove one or both of them from possiblePeers
		if !srcPeer.CanBeUsed(t.MaxPeers) || !dstPeer.CanBeUsed(t.MaxPeers) {
			newLength, oldLength := 0, len(possiblePeers)
			for i := 0; i < oldLength; i++ {
				agentCont := possiblePeers[i]
				peer := peerMap[agentCont.port]
				if peer.CanBeUsed(t.MaxPeers) {
					possiblePeers[newLength] = agentCont
					newLength++
				} else {
					delete(peerMap, agentCont.port)
				}
			}
			possiblePeers = possiblePeers[:newLength]
		}
	}

	// connecting all the nodes from the list
	success, failed, elapsed := connections.ConnectAll()
	fmt.Printf("Connecting finished. success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)
}

// Holds current number of connections for one peer, port id, agent and slice of all other possible connections (as port ids)
type rndToplogyPeer struct {
	connCount     int
	portID        int
	agent         ClusterAgent
	possibleConns []int
}

func newRndTopologyPeer(portID int, agent ClusterAgent, agents map[int]agentContainer) *rndToplogyPeer {
	possible := make([]int, 0, len(agents)-1)
	for otherPortID := range agents {
		if otherPortID != portID {
			possible = append(possible, otherPortID)
		}
	}
	return &rndToplogyPeer{
		connCount:     0,
		portID:        portID,
		possibleConns: possible,
		agent:         agent,
	}
}

func (tp *rndToplogyPeer) IncConnCount() {
	tp.connCount++
}

func (tp *rndToplogyPeer) RemovePortId(portID int) {
	for i := 0; i < len(tp.possibleConns); i++ {
		if tp.possibleConns[i] == portID {
			tp.RemoveIndex(i)
			break
		}
	}
}

func (tp *rndToplogyPeer) RemoveIndex(index int) {
	size := len(tp.possibleConns)
	tp.possibleConns[index] = tp.possibleConns[size-1]
	tp.possibleConns = tp.possibleConns[:size-1]
}

func (tp *rndToplogyPeer) PopRandomPortID() int {
	index := rand.Intn(len(tp.possibleConns))
	value := tp.possibleConns[index]
	tp.RemoveIndex(index)
	return value
}

func (tp rndToplogyPeer) CanBeUsed(max int) bool {
	return len(tp.possibleConns) > 0 && tp.connCount < max
}
