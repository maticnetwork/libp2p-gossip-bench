package cluster

import (
	"fmt"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
)

// Each non validator is connected to at least one validator
type SuperClusterTopology struct {
	ValidatorPeering    uint // number of connections between each validator to other validators. It can be less for some validators
	NonValidatorPeering uint // number of connections between each non validator to other non validators.
}

// Creates supercluster topology connections between peers
func (t SuperClusterTopology) MakeConnections(agents map[int]agent.Agent) {
	const connectionsNumber = 1000000
	validators, nonValidators := make([]agent.Agent, 0), make([]agent.Agent, 0)
	// create two list - one for validators and one for non validators
	for _, ac := range agents {
		if ac.IsValidator() {
			validators = append(validators, ac)
		} else {
			nonValidators = append(nonValidators, ac)
		}
	}

	// make connections between validators
	connections := make(connectionsList, 0)
	rTopology := RandomTopology{CreateRing: true, Count: connectionsNumber, MaxPeers: t.ValidatorPeering}
	rTopology.populate(&connections, validators)

	success, failed, elapsed := connections.ConnectAll()
	fmt.Printf("Connecting validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)

	if len(nonValidators) > 0 {
		// make connections between non validators
		connections = make(connectionsList, 0)
		rTopology := RandomTopology{CreateRing: true, Count: connectionsNumber, MaxPeers: t.NonValidatorPeering}
		rTopology.populate(&connections, nonValidators)

		success, failed, elapsed = connections.ConnectAll()
		fmt.Printf("Connecting non validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)

		// make connection between each non validator and exactly one validator
		connections = make(connectionsList, 0)
		for i, ac := range nonValidators {
			j := i % len(validators)
			connections.Add(ac, validators[j])
		}
		success, failed, elapsed = connections.ConnectAll()
		fmt.Printf("Connecting non validators to validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)
	}
}
