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
	MessageSize int           // size of sent message in bytes
}

func (c ConstantRateMessaging) Loop(ctx context.Context, agents []agent.Agent) (int64, int64) {
	var publishedCnt, failedCnt int64
	var wg sync.WaitGroup
	start := time.Now()

	// create time observable
	timeSubject := observer.NewSubject(start)
	go updateSubjectForRate(ctx, timeSubject, c.Rate)

	for _, a := range agents {
		// start waiting for each agent we've started
		wg.Add(1)
		go func(a agent.Agent) {
			defer wg.Done()
			stream := timeSubject.Observe()

			for {
				select {
				case <-ctx.Done():
					// kill or expiration signal received
					return
					// wait for update changes
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

	// wait for all started agents to shutdown
	wg.Wait()

	return publishedCnt, failedCnt
}

type HotstuffMessaging struct {
	LogDuration time.Duration // period of time for which loggs are considered useful
	MessageSize int           // size of sent message in bytes
}

func (h HotstuffMessaging) Loop(ctx context.Context, agents []agent.Agent) (int64, int64) {
	var publishedCnt, failedCnt int64
	var wg sync.WaitGroup
	start := time.Now()

	// create time observable
	timeSubject := observer.NewSubject(start)
	go updateSubjectForRate(ctx, timeSubject, time.Second)

	// get a random index, for a random leader agent
	rand.Seed(time.Now().UnixNano())
	randIdx := rand.Intn(len(agents))
	leader := agents[randIdx]
	// get rest of the agents
	rest := append(agents[:randIdx], agents[randIdx+1:]...)

	// leader sends messages on odd seconds
	wg.Add(1)
	go func(a agent.Agent) {
		defer wg.Done()
		stream := timeSubject.Observe()

		for {
			select {
			case <-ctx.Done():
				// kill or expiration signal received
				return
			case <-stream.Changes():
				// advance to next value
				stream.Next()
				currentTime := stream.Value().(time.Time)

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

	// rest of the agents send their messages on even seconds
	for _, a := range rest {
		wg.Add(1)
		go func(a agent.Agent) {
			defer wg.Done()
			stream := timeSubject.Observe()

			for {
				select {
				case <-ctx.Done():
					// kill or expiration signal received
					return
				case <-stream.Changes():
					// advance to next value
					stream.Next()
					currentTime := stream.Value().(time.Time)

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

	// wait for all started agents to shutdown
	wg.Wait()

	return publishedCnt, failedCnt
}

func onEvenSecond(t time.Time) bool {
	return t.Second()%2 == 0
}

func shouldAggregate(start time.Time, duration time.Duration) bool {
	msgTime := time.Now()
	return msgTime.Before(start.Add(duration))
}

// updateSubjectForRate is used to update created observer Subject for defined rate
func updateSubjectForRate(ctx context.Context, subject observer.Subject, rate time.Duration) {
	// create a ticker for a defined messaging rate
	tm := time.NewTicker(rate)
	defer tm.Stop()
	for {
		select {
		case <-ctx.Done():
			// kill or expiration signal received
			return
		case time := <-tm.C:
			// update subject with new time from ticker
			subject.Update(time)
		}
	}
}
