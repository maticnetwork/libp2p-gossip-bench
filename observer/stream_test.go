package observer

import (
	"testing"
	"time"
)

func TestStreamInitialValue(t *testing.T) {
	try := "yo"
	state := newState(try)
	stream := &stream{state: state}
	if val := stream.Value(); val == try {
		t.Errorf("Expected to get %#v but got %#v\n", try, val)
	}
}

func TestStreamUpdate(t *testing.T) {
	init := "yo"
	state := newState(init)
	stream := &stream{state: state}

	update := "po"
	updatedState := state.update(update)

	if val := stream.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}

	updatedState.update("ndoshta")
	if val := stream.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}
}

func TestStreamNext(t *testing.T) {
	init := "yo"
	state := newState(init)
	stream := &stream{state: state}

	update := "po"
	updatedState := state.update(update)

	if val := stream.Next(); val != update {
		t.Errorf("Expected to get %#v but got %#v\n", update, val)
	}

	newUpdate := "ndoshta"
	updatedState.update(newUpdate)

	if val := stream.Next(); val != newUpdate {
		t.Errorf("Expected to get %#v but got %#v\n", update, val)
	}
}

func TestStreamChanges(t *testing.T) {
	init := "yo"
	state := newState(init)
	stream := &stream{state: state}

	select {
	case <-stream.Changes():
		t.Errorf("Expecting nothing here")
	default:
	}

	update := "po"
	go func() {
		time.Sleep(1 * time.Second)
		state.update(update)
	}()

	select {
	case <-stream.Changes():
	// wait some period of time longer than 1 second needed for update
	// if this happens 1st, we have a problem
	case <-time.After(2 * time.Second):
		t.Errorf("Shold be changes before this")
	}

	// state done channel is closed at this point
	// we should still be able to receive from Changes
	select {
	case <-stream.Changes():
	default:
		t.Errorf("Should be changes here")
	}

	if val := stream.Next(); val != update {
		t.Errorf("Expected to get %#v but got %#v\n", update, val)
	}

	select {
	case <-stream.Changes():
		t.Errorf("Should be no changes here")
	default:
	}
}
