package cluster

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
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

	for _, a := range agents {
		// start waiting for each agent we've started
		wg.Add(1)
		go func(a agent.Agent) {
			tm := time.NewTicker(c.Rate)
			defer func() {
				tm.Stop()
				defer wg.Done()
			}()

			for {
				select {
				case <-ctx.Done():
					// kill or expiration signal received
					return
				case <-tm.C:
					// log message, used for stats aggregation,
					// should be logged only during specified benchmark log duration
					err := a.SendMessage(c.MessageSize, shouldAggregate(start, c.LogDuration))
					if err != nil {
						atomic.AddInt64(&publishedCnt, 1)
					} else {
						atomic.AddInt64(&failedCnt, 1)
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

	// start ticker for leader and the rest of agents
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	leaderChan, restChan := doubleTicker(tick.C)

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
		for {
			select {
			case <-ctx.Done():
				// kill or expiration signal received
				return
			case <-leaderChan:
				// log message, used for stats aggregation,
				// should be logged only during specified benchmark log duration
				err := a.SendMessage(h.MessageSize, shouldAggregate(start, h.LogDuration))
				if err != nil {
					atomic.AddInt64(&publishedCnt, 1)
				} else {
					atomic.AddInt64(&failedCnt, 1)
				}
			}
		}

	}(leader)

	// rest of the agents send their messages on even seconds
	for _, a := range rest {
		wg.Add(1)
		go func(a agent.Agent) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					// kill or expiration signal received
					return
				case <-restChan:
					err := a.SendMessage(h.MessageSize, shouldAggregate(start, h.LogDuration))
					if err != nil {
						atomic.AddInt64(&publishedCnt, 1)
					} else {
						atomic.AddInt64(&failedCnt, 1)
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

// doubleTicker is used to determine the rhythm of sent messages
// channel for a leader agent receives messages on odd seconds
// chanel for the rest of agents receives messages on even seconds
func doubleTicker(c <-chan time.Time) (chan time.Time, chan time.Time) {
	leadChan := make(chan time.Time)
	restChan := make(chan time.Time)
	go func() {
		for t := range c {
			if onEvenSecond(t) {
				restChan <- t
			} else {
				leadChan <- t
			}
		}
	}()
	return leadChan, restChan
}

func shouldAggregate(start time.Time, duration time.Duration) bool {
	msgTime := time.Now()
	return msgTime.Before(start.Add(duration))

}
