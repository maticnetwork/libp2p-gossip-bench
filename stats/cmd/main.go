package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

var (
	durations = []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second}
	filePath  = "/tmp/agents1.log"
)

func main() {
	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("error opening file: %v\n", err)
		return
	}
	defer func() {
		err := f.Close()
		if err != nil {
			fmt.Printf("Could not close file. Error: %s", err)
		}
	}()

	result := make(map[int]stats)
	s := bufio.NewScanner(f)
	var logLine Log
	for s.Scan() {
		err := json.Unmarshal(s.Bytes(), &logLine)
		if err != nil {
			fmt.Printf("error unmarshaling log: %s", err)
			return
		}
		addStatistics(logLine, result)
	}

	printStats(result)
}

func addStatistics(logLine Log, result map[int]stats) {
	// todo add better handling of different kind of messages
	if logLine.Msg == "message sent" {
		sentTime := logLine.Time
		if r, ok := result[logLine.Data]; ok {
			r.msgSentTime = sentTime
		} else {
			m := make(map[time.Duration]int)
			result[logLine.Data] = stats{
				msgSentTime:        sentTime,
				data:               logLine.Data,
				durationStatistics: m,
			}
		}
	} else if logLine.Msg == "message received" {
		receivedTime := logLine.Time
		if r, ok := result[logLine.Data]; ok {
			total := r.totalCount
			r.totalCount = total + 1
			duration := receivedTime.Sub(r.msgSentTime)
			if r.lastMsgReceivedTime.Before(receivedTime) {
				r.lastMsgReceivedTime = receivedTime
			}
			result[logLine.Data] = r
			for _, d := range durations {
				if duration <= d {
					incrementNodesCount(r.durationStatistics, d)
				}
			}
		}
	}
}

func incrementNodesCount(durationStatistics map[time.Duration]int, duration time.Duration) {
	if k, ok := durationStatistics[duration]; ok {
		durationStatistics[duration] = k + 1
	} else {
		durationStatistics[duration] = 1
	}
}

type Log struct {
	Data  int       `json:"data"`
	Msg   string    `json:"msg"`
	From  string    `json:"from"`
	Peer  string    `json:"peer"`
	Topic string    `json:"topic"`
	Time  time.Time `json:"time"`
}

type stats struct {
	totalCount          int
	data                int // todo this can be messageID
	msgSentTime         time.Time
	lastMsgReceivedTime time.Time
	durationStatistics  map[time.Duration]int
}

func (s stats) String() string {
	return fmt.Sprintf("Data: %d\nTotal:%d\nSentTime: %s\nLastReceivedTime: %s\nMaxDuration: %.2f seconds\n", s.data, s.totalCount, s.msgSentTime, s.lastMsgReceivedTime, s.lastMsgReceivedTime.Sub(s.msgSentTime).Seconds()) //, s.receivedMsgCount)
}

func printStats(result map[int]stats) {
	for _, stats := range result {
		fmt.Printf("Statistics:\n%v", stats)
		for _, d := range durations {
			fmt.Printf("Threshold: <= %vseconds, count: %d, percentage: %.2f%%\n", d.Seconds(), stats.durationStatistics[d], float64(stats.durationStatistics[d])/float64(stats.totalCount)*100)
		}
	}
}
