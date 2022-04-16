package observer

// Stream acts as a list of values that subject is being updated to.
// On each subject update, that value is appended to the stream, in
// exact order they happen.
// The value is ditched once we move along the stream.
// Stream is not goroutine safe.
type Stream interface {
	// Value for the current value in this stream
	Value() interface{}

	// Changes gives a channel that is closed when a new value is in the stream
	Changes() chan struct{}

	// Next pushes on this stream to the next state.
	// Should be called only when Changes channel is closed,
	// and used only when we want more controll
	Next() interface{}

	// HasNext tells whether stream has a new value or not
	HasNext() bool

	// WaitNext is blocked until Changes is closed,
	// proceeds the stream to the next state and returns
	// the current value
	WaitNext() interface{}
}

type stream struct {
	state *state
}

func (s *stream) Value() interface{} {
	return s.state.value
}

func (s *stream) Changes() chan struct{} {
	return s.state.done
}

func (s *stream) Next() interface{} {
	s.state = s.state.next
	return s.state.value
}

func (s *stream) HasNext() bool {
	select {
	case <-s.state.done:
		return true
	default:
		return false
	}
}

func (s *stream) WaitNext() interface{} {
	<-s.state.done
	s.state = s.state.next
	return s.state.value
}
