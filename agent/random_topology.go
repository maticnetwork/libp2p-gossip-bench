package agent

import (
	"fmt"
	"math/rand"
)

type RandomTopology struct {
	MaxPeers   uint // maximum number of connected peers
	Count      uint // count of connections in topology
	CreateRing bool // if true there will always be linear connection beetwen nodes
}

// Creates random topology connections between peers
func (t RandomTopology) MakeConnections(agents map[int]agentContainer) {
	connections := NewConnectionsList()

	// create slice of agents and then populate random connection
	possiblePeers := make([]agentContainer, 0, len(agents)) // list of all possible peers that can be choosen as source in one connection
	for _, value := range agents {
		possiblePeers = append(possiblePeers, value)
	}
	t.populate(&connections, possiblePeers)

	// connecting all the nodes from the list
	success, failed, elapsed := connections.ConnectAll()
	fmt.Printf("Connecting finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)
}

func (t RandomTopology) populate(connections *connectionsList, peers []agentContainer) {
	count := t.Count
	possiblePeers := make([]agentContainer, len(peers))  // this holds list of all possible peers in one iteration
	peerMap := make(map[int]*rndToplogyPeer, len(peers)) // map portID -> *rndToplogyPeer

	// init structures
	copy(possiblePeers, peers)
	for _, agentCont := range peers {
		peerMap[agentCont.port] = newRndTopologyPeer(agentCont.port, agentCont.agent, peers)
	}

	// create ring topology between nodes if specified
	if t.CreateRing {
		end := len(possiblePeers)
		if end <= 2 { // otherwise there would be 1-2 and 2-1 or 1-1 connections
			end--
		}
		for i := 0; i < end && count > 0; i++ {
			j := (i + 1) % len(possiblePeers)
			src, dst := possiblePeers[i], possiblePeers[j]
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
}

// Holds current number of connections for one peer, port id, agent and slice of all other possible connections (as port ids)
type rndToplogyPeer struct {
	connCount     uint
	portID        int
	agent         Agent
	possibleConns []int
}

func newRndTopologyPeer(portID int, agent Agent, agents []agentContainer) *rndToplogyPeer {
	possible := make([]int, 0, len(agents)-1)
	for _, agent := range agents {
		if agent.port != portID {
			possible = append(possible, agent.port)
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

func (tp rndToplogyPeer) CanBeUsed(max uint) bool {
	return len(tp.possibleConns) > 0 && tp.connCount < max
}
