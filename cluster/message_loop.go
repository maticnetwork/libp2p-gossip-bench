package cluster

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	"github.com/maticnetwork/libp2p-gossip-bench/observer"
)

type Messaging interface {
	Loop(ctx context.Context, agents []agent.Agent) (publishedCnt, failedCnt int64)
}

type ConstantRateMessaging struct {
	Rate        time.Duration // rate at which agents will fire messages
	LogDuration time.Duration // period of time for which loggs are considered useful
	Timeout     time.Duration // period of time at which messaging has to stop
	MessageSize int           // size of sent message in bytes
}

func (c ConstantRateMessaging) Loop(ctx context.Context, agents []agent.Agent) (int64, int64) {
	var publishedCnt, failedCnt int64
	var start time.Time
	var wg sync.WaitGroup

	// we need additional channel to stop all running routines
	// after timeout timer expires
	// context.WithTimeout can't be trusted here
	stopCh := make(chan struct{})

	// create time observable
	timeSubject := observer.NewSubject(start)

	for _, a := range agents {
		// add each agent into wait group
		wg.Add(1)
		go func(a agent.Agent) {
			defer wg.Done()
			stream := timeSubject.Observe()

			for {
				select {
				case <-ctx.Done():
				case <-stopCh:
					// kill or stop signal received
					return
					// wait for stream update changes
				case <-stream.Changes():
					// advance to next value
					stream.Next()
					// log message, used for stats aggregation,
					// should be logged only during specified benchmark log duration
					err := a.SendMessage(c.MessageSize, shouldAggregate(start, c.LogDuration))
					if err != nil {
						atomic.AddInt64(&failedCnt, 1)
					} else {
						atomic.AddInt64(&publishedCnt, 1)
					}
				}
			}
		}(a)
	}

	// start measuring time for messaging loop
	start = time.Now()
	// update observer subject periodically
	startUpdatingSubject(ctx, timeSubject, c.Rate, c.Timeout, stopCh)
	// wait for all agents to finish
	wg.Wait()

	return atomic.LoadInt64(&publishedCnt), atomic.LoadInt64(&failedCnt)
}

type HotstuffMessaging struct {
	LogDuration time.Duration // period of time for which loggs are considered useful
	Timeout     time.Duration // period of time at which messaging has to stop
	MessageSize int           // size of sent message in bytes
}

func (h HotstuffMessaging) Loop(ctx context.Context, agents []agent.Agent) (int64, int64) {
	var publishedCnt, failedCnt int64
	var start time.Time
	var wg sync.WaitGroup

	// we need additional channel to stop all running routines
	// after timeout timer expires
	// context.WithTimeout can't be trusted here
	stopCh := make(chan struct{})

	// create time observable
	timeSubject := observer.NewSubject(start)

	// get a random index, for a random leader agent
	rand.Seed(time.Now().UnixNano())
	randIdx := rand.Intn(len(agents))
	leader := agents[randIdx]
	// get rest of the agents
	rest := append(agents[:randIdx], agents[randIdx+1:]...)

	// Leader sends messages on odd seconds
	wg.Add(1) // add leader in wait group
	go func(a agent.Agent) {
		defer wg.Done()
		stream := timeSubject.Observe()

		for {
			select {
			case <-ctx.Done():
			case <-stopCh:
				// kill or stop signal received
				return
				// wait for stream update changes
			case <-stream.Changes():
				// advance to next value
				stream.Next()
				currentTime := stream.Value().(time.Duration)

				// check for odd second
				if !onEvenSecond(currentTime) {
					// log message, used for stats aggregation,
					// should be logged only during specified benchmark log duration
					err := a.SendMessage(h.MessageSize, shouldAggregate(start, h.LogDuration))
					if err != nil {
						atomic.AddInt64(&failedCnt, 1)
					} else {
						atomic.AddInt64(&publishedCnt, 1)
					}
				}
			}
		}

	}(leader)

	// Rest of the agents send their messages on even seconds
	for _, a := range rest {
		wg.Add(1)
		go func(a agent.Agent) {
			defer wg.Done()
			stream := timeSubject.Observe()

			for {
				select {
				case <-ctx.Done():
				case <-stopCh:
					// kill or stop signal received
					return
					// wait for stream update changes
				case <-stream.Changes():
					// advance to next value
					stream.Next()
					currentTime := stream.Value().(time.Duration)

					// check for even second
					if onEvenSecond(currentTime) {
						// log message, used for stats aggregation,
						// should be logged only during specified benchmark log duration
						err := a.SendMessage(h.MessageSize, shouldAggregate(start, h.LogDuration))
						if err != nil {
							atomic.AddInt64(&failedCnt, 1)
						} else {
							atomic.AddInt64(&publishedCnt, 1)
						}
					}
				}
			}
		}(a)
	}

	// start measuring time for messaging loop
	start = time.Now()
	// update observer subject periodically on each second
	startUpdatingSubject(ctx, timeSubject, time.Second, h.Timeout, stopCh)
	// wait for all agents to finish
	wg.Wait()

	return atomic.LoadInt64(&publishedCnt), atomic.LoadInt64(&failedCnt)
}

func onEvenSecond(d time.Duration) bool {
	seconds := d.Seconds()
	return int(seconds)%2 == 0
}

func shouldAggregate(start time.Time, duration time.Duration) bool {
	msgTime := time.Now()
	return msgTime.Before(start.Add(duration))
}

// startUpdatingSubject is used to update created observer Subject for defined rate of time
// will block until context is canceled
func startUpdatingSubject(ctx context.Context, subject observer.Subject, rate, timeout time.Duration, done chan struct{}) {
	// create a ticker for a defined messaging rate
	tm := time.NewTicker(rate)
	start := time.Now()
	end := time.NewTimer(timeout)
	defer func() {
		tm.Stop()
		end.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
		case <-end.C:
			// kill or end signal received
			// signal done to running agents
			close(done)
			return
		case time := <-tm.C:
			elapsed := time.Sub(start)
			// update subject with new elapsed duration from ticker
			subject.Update(elapsed)
		}
	}
}
