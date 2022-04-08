package utils

import (
	"sync"
	"sync/atomic"
	"time"
)

const itemsPerRoutine = 2
const maxRoutines = 1000

// Executes provided function fn in multiple go routines exactly itemCount times
func MultiRoutineRunner(itemsCount int, fn func(index int) error) (int, int, time.Duration) {
	startTime, success, failed := time.Now(), int32(0), int32(0)
	routinesCount := (itemsCount + itemsPerRoutine - 1) / itemsPerRoutine
	if routinesCount > maxRoutines {
		routinesCount = maxRoutines
	}
	cntPerRoutine := (itemsCount + routinesCount - 1) / routinesCount

	wg := sync.WaitGroup{}
	wg.Add(routinesCount)

	for i := 0; i < routinesCount; i++ {
		cnt, offset := cntPerRoutine, i*cntPerRoutine
		if cnt+offset > itemsCount {
			cnt = itemsCount - offset
		}

		go func(offset, cntConnections int) {
			for i := 0; i < cntConnections; i++ {
				err := fn(offset + i)
				if err != nil {
					atomic.AddInt32(&failed, 1)
				} else {
					atomic.AddInt32(&success, 1)
				}
			}

			wg.Done()
		}(offset, cnt)
	}

	wg.Wait()
	return int(success), int(failed), time.Since(startTime)
}
