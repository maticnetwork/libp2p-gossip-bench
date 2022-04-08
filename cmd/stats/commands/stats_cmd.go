package commands

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/cli"
)

// StartCommand is a struct containing data for running framework
type StatsCommand struct {
	UI cli.Ui

	filePath    string
	maxDuration int
}

// Help implements the cli.Command interface
func (fc *StatsCommand) Help() string {
	return `Command runs the libp2p framework based on provided configuration (node count, validator count).

    Usage: start -path={filePath} -maxDuration={25s}

    Options:	
    -path        - Path to file with logs
    -maxDuration - Maximum duration in seconds for statistics`
}

// Synopsis implements the cli.Command interface
func (fc *StatsCommand) Synopsis() string {
	return "Starts the statistics"
}

// Run implements the cli.Command interface and runs the command
func (fc *StatsCommand) Run(args []string) int {
	flagSet := fc.NewFlagSet()
	err := flagSet.Parse(args)
	if err != nil {
		fc.UI.Error(err.Error())
		return 1
	}

	fc.UI.Info("Starting statistics ...")
	fc.UI.Info(fmt.Sprintf("File path: %v", fc.filePath))
	fc.UI.Info(fmt.Sprintf("Max duration in seconds: %v", fc.maxDuration))

	if strings.TrimSpace(fc.filePath) == "" {
		fc.UI.Info("Empty file path submitted")
		return 1
	}

	RunStats(fc.filePath, fc.maxDuration)
	fc.UI.Info("Stats executed")

	return 0
}

// NewFlagSet implements the interface and creates a new flag set for command arguments
func (fc *StatsCommand) NewFlagSet() *flag.FlagSet {
	flagSet := flag.NewFlagSet("libp2p-stats", flag.ContinueOnError)
	flagSet.StringVar(&fc.filePath, "path", "", "Path to file with logs")
	flagSet.IntVar(&fc.maxDuration, "duration", 25, "Maximum duration in seconds for statistics")

	return flagSet
}

var (
	durations []time.Duration
)

func RunStats(filePath string, maxDurationInSeconds int) {
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
	for i := 1; i <= maxDurationInSeconds; i++ {
		durations = append(durations, time.Duration(i)*time.Second)
	}

	result := make(map[string]stats)
	s := bufio.NewScanner(f)
	var logLine Log
	for s.Scan() {
		err := json.Unmarshal(s.Bytes(), &logLine)
		if err != nil {
			fmt.Printf("error unmarshaling log: %s\n", err)
			return
		}
		addStatistics(logLine, result)
	}

	printStats(result)
}
func addStatistics(logLine Log, result map[string]stats) {
	if logLine.Direction == "sent" {
		sentTime := logLine.Time
		if r, ok := result[logLine.MsgID]; ok {
			r.msgSentTime = sentTime
		} else {
			m := make(map[time.Duration]int)
			result[logLine.MsgID] = stats{
				msgSentTime:        sentTime,
				msgID:              logLine.MsgID,
				durationStatistics: m,
			}
		}
	} else if logLine.Direction == "received" {
		receivedTime := logLine.Time
		if r, ok := result[logLine.MsgID]; ok {
			total := r.totalNodesCount
			r.totalNodesCount = total + 1
			duration := receivedTime.Sub(r.msgSentTime)
			if r.lastMsgReceivedTime.Before(receivedTime) {
				r.lastMsgReceivedTime = receivedTime
			}
			result[logLine.MsgID] = r
			for _, d := range durations {
				if duration <= d {
					incrementNodesCount(r.durationStatistics, d)
				}
			}
		}
	}
}

type Log struct {
	MsgID     string    `json:"msgID"`
	Direction string    `json:"direction"`
	Msg       string    `json:"msg"`
	From      string    `json:"from"`
	Peer      string    `json:"peer"`
	Topic     string    `json:"topic"`
	Time      time.Time `json:"time"`
}

type stats struct {
	totalNodesCount     int
	msgID               string
	msgSentTime         time.Time
	lastMsgReceivedTime time.Time
	durationStatistics  map[time.Duration]int
}

func (s stats) String() string {
	return fmt.Sprintf("MessageID: %s\nTotalNodes:%d\nSentTime: %s\nLastReceivedTime: %s\nAllNodesRecivedMsgIn: %.2f second(s)\n",
		s.msgID, s.totalNodesCount, s.msgSentTime, s.lastMsgReceivedTime, s.lastMsgReceivedTime.Sub(s.msgSentTime).Seconds())
}

func incrementNodesCount(durationStatistics map[time.Duration]int, duration time.Duration) {
	if k, ok := durationStatistics[duration]; ok {
		durationStatistics[duration] = k + 1
	} else {
		durationStatistics[duration] = 1
	}
}

func printStats(result map[string]stats) {
	for _, stats := range result {
		maxDuration := stats.lastMsgReceivedTime.Sub(stats.msgSentTime)
		fmt.Printf("Statistics:\n%v", stats)
		for _, d := range durations {
			if d <= maxDuration {
				fmt.Printf("Threshold: <= %vsecond(s), NodeCount: %d, Percentage: %.2f%%\n",
					d.Seconds(), stats.durationStatistics[d], float64(stats.durationStatistics[d])/float64(stats.totalNodesCount)*100)
			}
		}
	}
}

func GetStatsCommands() map[string]cli.CommandFactory {
	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}
	return map[string]cli.CommandFactory{
		"start": func() (cli.Command, error) {
			return &StatsCommand{
				UI: ui,
			}, nil
		},
	}
}
