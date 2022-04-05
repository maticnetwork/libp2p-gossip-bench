package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	durationThreshold := 2 * time.Second
	filePath := "/tmp/agents_2022-04-05T16:06:23+02:00.log" //"/tmp/agents1.log" //"/tmp/agents.log"

	f, err := os.Open(filePath)
	if err != nil {
		log.Printf("error opening file: %v\n", err)
		return
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	result := make(map[int]stats)
	for s.Scan() {
		logLine := Log{}
		err := json.Unmarshal(s.Bytes(), &logLine)
		if err != nil {
			log.Printf("error unmarshaling log: %s", err)
			return
		}

		if logLine.Msg == "message sent" {
			sentTime := logLine.Time
			if r, ok := result[logLine.Data]; ok {
				r.msgSentTime = sentTime
			} else {
				result[logLine.Data] = stats{msgSentTime: sentTime, data: logLine.Data}
			}
		} else if logLine.Msg == "message received" {
			receivedTime := logLine.Time
			if r, ok := result[logLine.Data]; ok {
				if receivedTime.Sub(r.msgSentTime) <= durationThreshold {
					if receivedTime.After(r.lastMsgReceivedTime) {
						r.lastMsgReceivedTime = receivedTime
					}
					r.receivedMsgCount++
					result[logLine.Data] = r
				}
			} else {
				result[logLine.Data] = stats{
					receivedMsgCount:    1,
					lastMsgReceivedTime: receivedTime,
				}
			}
		}

		log.Printf("Log %+v\n", logLine)
	}
	for k, v := range result {
		log.Printf("Data: %d, %s\n", k, v)
		log.Printf("Percentage of nodes received message %v: %.2f\n", v.data, float64(v.receivedMsgCount)/8*100)
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
	data                int
	msgSentTime         time.Time
	lastMsgReceivedTime time.Time
	receivedMsgCount    int
}

func (s stats) String() string {
	return fmt.Sprintf("SentTime: %s, LastReceivedTime: %s, Diff: %dms, NodesReceived: %d", s.msgSentTime, s.lastMsgReceivedTime, s.lastMsgReceivedTime.Sub(s.msgSentTime).Milliseconds(), s.receivedMsgCount)
}
