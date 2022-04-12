package agent

import (
	"fmt"
)

// Each non validator is connected to at least one validator
type SuperClusterTopology struct {
	ValidatorPeering    uint // number of connections between each validator to other validators. It can be less for some validators
	NonValidatorPeering uint // number of connections between each non validator to other non validators.
}

// Creates supercluster topology connections between peers
func (t SuperClusterTopology) MakeConnections(agents map[int]agentContainer) {
	validators, nonValidators := make([]agentContainer, 0), make([]agentContainer, 0)
	// create two list - one for validators and one for non validators
	for _, ac := range agents {
		if ac.isValidator {
			validators = append(validators, ac)
		} else {
			nonValidators = append(nonValidators, ac)
		}
	}

	// make connections between validators
	connections := NewConnectionsList()
	RandomTopology{CreateRing: true, Count: 1000000, MaxPeers: t.ValidatorPeering}.populate(&connections, validators)
	success, failed, elapsed := connections.ConnectAll()
	fmt.Printf("Connecting validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)

	if len(nonValidators) > 0 {
		// make connections between non validators
		connections.Clear()
		RandomTopology{CreateRing: true, Count: 1000000, MaxPeers: t.NonValidatorPeering}.populate(&connections, nonValidators)
		success, failed, elapsed = connections.ConnectAll()
		fmt.Printf("Connecting non validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)

		// make connection between each non validator and exactly one validator
		connections.Clear()
		for i, ac := range nonValidators {
			j := i % len(validators)
			connections.Add(ac.agent, validators[j].agent)
		}
		success, failed, elapsed = connections.ConnectAll()
		fmt.Printf("Connecting non validators to validators finished. Success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)
	}
}
