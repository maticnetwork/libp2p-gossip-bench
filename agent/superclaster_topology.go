package agent

import (
	"fmt"
	"math/rand"
)

type SuperClusterTopology struct {
	ValidatorPeering    uint // number of connections between each validator to other validators. It can be less for some peers
	NonValidatorPeering uint // number of connections between each non validator to some validator
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

	connections := NewConnectionsList()

	// Connect each non validator to some random validator.
	// Be sure that number of connections is evenly distributed among validators
	possible := make([]agentContainer, len(validators))
	copy(possible, validators)
	possibleCnt := len(validators)
	for _, ac := range nonValidators {
		for i := uint(0); i < t.NonValidatorPeering; i++ {
			index := rand.Intn(possibleCnt)
			connections.Add(ac.agent, possible[index].agent)
			if possibleCnt == 1 {
				possibleCnt = len(validators)
			} else {
				possibleCnt--
				possible[index], possible[possibleCnt] = possible[possibleCnt], possible[index] // swap possible validators
			}
		}
	}

	connectionsCount := make([]uint, len(validators))
	// make connections between validators
	for i := 0; i < len(validators)-1; i++ {
		for j := i + 1; j < len(validators) && connectionsCount[i] < t.ValidatorPeering; j++ {
			connections.Add(validators[i].agent, validators[j].agent)
			connectionsCount[i]++
			connectionsCount[j]++
		}
	}

	// connecting all the nodes from the list
	success, failed, elapsed := connections.ConnectAll()
	fmt.Printf("Connecting finished. success: %d, failed: %d. Elapsed: %v\n", success, failed, elapsed)
}
