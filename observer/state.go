package observer

// The state object contains information about every event.
// State objects are managed using a singly linked list structure.
// Meaning that every state point to the next one.
// For a newly raised event a new state object is appended to the
// list and the channel of the previous state is closed.
// This way all observers get notified that the previous state is stale.
type state struct {
	value interface{}
	next  *state
	done  chan struct{}
}

func newState(value interface{}) *state {
	return &state{
		value: value,
		done:  make(chan struct{}),
	}
}

func (s *state) update(value interface{}) *state {
	s.next = newState(value)
	close(s.done)
	return s.next
}
