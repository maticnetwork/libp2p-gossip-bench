package observer

import "sync"

// Subject is intented to be continuosly updated by one or more publishers.
// Subject is fully goroutine safe.
type Subject interface {
	// Update sets a new value for this subject
	Update(value interface{})

	// Observe is used to watch over a Stream of values for this subject
	Observe() Stream
}

type subject struct {
	sync.RWMutex
	state *state
}

func NewSubject(value interface{}) Subject {
	return &subject{state: newState(value)}
}

func (s *subject) Update(value interface{}) {
	s.Lock()
	defer s.Unlock()
	s.state = s.state.update(value)
}

func (s *subject) Observe() Stream {
	s.RLock()
	defer s.RUnlock()
	return &stream{state: s.state}
}
