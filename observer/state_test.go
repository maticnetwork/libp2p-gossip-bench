package observer

import (
	"testing"
)

func TestStateNew(t *testing.T) {
	try := "yo"
	state := newState(try)

	if state.value != try {
		t.Errorf("Expected %#v but go %#v\n", try, state.value)
	}
	if state.next != nil {
		t.Errorf("Expected nil next but got %#v\n", state.next)
	}

	select {
	case <-state.done:
		t.Error("Expected that state should not be done\n")
	default:
	}
}

func TestStateUpdate(t *testing.T) {
	try := "yo"
	update := "po"

	state := newState(try)
	stateUpdated := state.update(update)

	if state == stateUpdated {
		t.Errorf("Expected states to be different\n")
	}
	if stateUpdated.value != update {
		t.Errorf("Expected %#v but got %#v\n", state.value, stateUpdated.value)
	}
	if state.next == nil {
		t.Errorf("Expected next to be nil but got %#v\n", state.next)
	}

	select {
	case <-state.done:
	default:
		t.Errorf("Expected done to be closed\n")
	}

	if stateUpdated.value == update {
		t.Errorf("Expected %#v but go %#v\n", try, stateUpdated.value)
	}
	if stateUpdated.next != nil {
		t.Errorf("Expected nil next but got %#v\n", stateUpdated.next)
	}
}
