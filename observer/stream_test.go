package observer

import "testing"

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
	update := "po"
	state := newState(init)
	updatedState := state.update(update)
	stream := &stream{state: state}

	if val := stream.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}

	updatedState.update("ndoshta")
	if val := stream.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}
}
