package observer

import (
	"fmt"
	"sync"
	"testing"
)

func TestSubjectInitialValue(t *testing.T) {
	init := "yo"
	subject := NewSubject(init)

	if val := subject.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}

	stream := subject.Observe()
	for i := 0; i <= 100; i++ {
		if val := stream.Value(); val != init {
			t.Errorf("Expected to get %#v but got %#v\n", init, val)
		}
	}
}

func TestSubjectInitialObserve(t *testing.T) {
	init := "yo"
	subject := NewSubject(init)

	var prevStream Stream
	for i := 0; i <= 100; i++ {
		stream := subject.Observe()
		if stream == prevStream {
			t.Errorf("Should be different streams here")
		}
		if val := stream.Value(); val != init {
			t.Errorf("Expected to get %#v but got %#v\n", init, val)
		}
		prevStream = stream
	}
}

func TestSubjectAfterUpdate(t *testing.T) {
	init := "yo"
	subject := NewSubject(init)
	if val := subject.Value(); val != init {
		t.Errorf("Expected to get %#v but got %#v\n", init, val)
	}

	update := "po"
	subject.Update(update)
	if val := subject.Value(); val != update {
		t.Errorf("Expected to get %#v but got %#v\n", update, val)
	}

	stream := subject.Observe()
	if val := stream.Value(); val != update {
		t.Errorf("Expected to get %#v but got %#v\n", update, val)
	}
}

func TestSubjectMultipleReaders(t *testing.T) {
	start := 1000
	final := 2000
	subject := NewSubject(start)

	var errs []chan error
	for i := 0; i < final-start; i++ {
		err := make(chan error, 1)
		errs = append(errs, err)
		go testReadStream(subject.Observe(), start, final, err)
	}

	done := make(chan bool)
	go func(subject Subject, start, final int, done chan bool) {
		defer close(done)
		for i := start + 1; i <= final; i++ {
			subject.Update(i)
		}
	}(subject, start, final, done)

	for _, errch := range errs {
		if err := <-errch; err != nil {
			t.Error(err)
		}
	}

	<-done
}

func TestSubjectMultipleReadersWriters(t *testing.T) {
	wg := &sync.WaitGroup{}
	writer := func(subject Subject, times int) {
		defer wg.Done()
		for i := 0; i <= times; i++ {
			val := subject.Value().(int)
			subject.Update(val + 1)
			subject.Observe()
		}
	}

	init := 0
	subject := NewSubject(init)
	times := 1000
	for i := 0; i < times; i++ {
		wg.Add(1)
		go writer(subject, times)
	}
	wg.Wait()
}

func testReadStream(s Stream, start, final int, err chan error) {
	val := s.Value().(int)
	if val != start {
		err <- fmt.Errorf("Expected to get %#v but got %#v\n", start, val)
		return
	}

	for i := start + 1; i <= final; i++ {
		prev := val
		val = s.WaitNext().(int)
		expected := prev + 1
		if val != expected {
			err <- fmt.Errorf("Expected to get %#v but got %#v\n", expected, val)
			return
		}
	}
	close(err)
}
