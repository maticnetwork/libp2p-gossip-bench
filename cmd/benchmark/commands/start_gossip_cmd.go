package commands

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	"github.com/maticnetwork/libp2p-gossip-bench/cluster"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"github.com/mitchellh/cli"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	linear       = "linear"
	random       = "random"
	superCluster = "super-cluster"
	constantRate = "constant-rate"
	hotStuff     = "hotstuff"
)

const (
	IpString                 = "127.0.0.1"
	outputFileDirectory      = "/tmp"
	RandomTopologyCreateRing = true
)

type GossipParameters struct {
	nodeCount          int
	validatorCount     int
	topology           string
	messaging          string
	messageRate        int
	benchDuration      int
	benchDowntime      int
	messageSize        int
	peeringDegree      int
	nonValidatorDegree int // used for super-cluster topology
	connectionCount    int // used for random topology
	startingPort       int
}

//StartGossipCommand is a struct containing data for running framework
type StartGossipCommand struct {
	UI     cli.Ui
	Params GossipParameters
}

// Help implements the cli.Command interface
func (fc *StartGossipCommand) Help() string {
	return `Command runs the libp2p framework based on provided configuration (node count, validator count, ).

    Usage: start -nodes={numberOfNodes} -validators={numberOfValidators} -topology={topologyType(linear,random, super-cluster)} -rate={messagesRate} -size={messageSize}

    Options:	
    -nodes                 - Count of nodes
    -validators            - Count of validators
    -topology              - Topology of the nodes (linear, random, super-cluster)
    -rate                  - Message rate of a node in milliseconds
    -duration              - Guaranteed benchmark duration in seconds for which the results will be logged
	-downtime	           - Period of time in seconds at the end of benchmark for which logs will be discarded 
    -size                  - Size of a transmitted message
	-degree                - Peering degree: count of directly connected peers
	-non-validator-degree  - Peering degree: count of directly connected non-validator peers (super-cluster only)
	-connection-count      - Number of connections in random topology
	-messaging			   - Messaging syncronization mechanism (constant rate, hotstuff)`
}

// Synopsis implements the cli.Command interface
func (fc *StartGossipCommand) Synopsis() string {
	return "Starts the libp2p framework"
}

// Run implements the cli.Command interface and runs the command
func (fc *StartGossipCommand) Run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	flagSet := fc.NewFlagSet()
	err := flagSet.Parse(args)
	if err != nil {
		fc.UI.Error(err.Error())
		return 1
	}

	if fc.Params.topology != superCluster {
		fc.Params.nonValidatorDegree = -1
	}
	if fc.Params.topology != random {
		fc.Params.connectionCount = -1
	}

	var topology cluster.Topology
	switch fc.Params.topology {
	case linear:
		fc.Params.peeringDegree = 2
		topology = cluster.LinearTopology{}
	case random:
		topology = cluster.RandomTopology{
			CreateRing: RandomTopologyCreateRing,
			MaxPeers:   uint(fc.Params.peeringDegree),
			Count:      uint(fc.Params.connectionCount),
		}
	case superCluster:
		topology = cluster.SuperClusterTopology{
			ValidatorPeering:    uint(fc.Params.peeringDegree),
			NonValidatorPeering: uint(fc.Params.nonValidatorDegree),
		}
	default:
		fc.UI.Info(fmt.Sprintf("Unknown topology %s submitted\n", fc.Params.topology))
		return 1
	}

	// initialize intervals
	benchDuration := time.Duration(fc.Params.benchDuration) * time.Second
	benchDowntime := time.Duration(fc.Params.benchDowntime) * time.Second
	// timeout for the whole benchmark should encorporate defined downtime
	benchTimeout := benchDuration + benchDowntime

	var messaging cluster.Messaging
	switch fc.Params.messaging {
	case constantRate:
		msgRate := time.Duration(fc.Params.messageRate) * time.Millisecond
		messaging = cluster.ConstantRateMessaging{
			Rate:        msgRate,
			LogDuration: benchDuration,
			MessageSize: fc.Params.messageSize,
		}
	case hotStuff:
		messaging = cluster.HotstuffMessaging{
			LogDuration: benchDuration,
			MessageSize: fc.Params.messageSize,
		}
	default:
		fc.UI.Info(fmt.Sprintf("Unknown messaging mechanism %s submitted\n", fc.Params.messaging))
		return 1
	}

	fc.UI.Info("Starting libp2p benchmark ...")
	fc.UI.Info(fmt.Sprintf("Node count: %v", fc.Params.nodeCount))
	fc.UI.Info(fmt.Sprintf("Validator count: %v", fc.Params.validatorCount))
	fc.UI.Info(fmt.Sprintf("Chosen topology: %s", fc.Params.topology))
	fc.UI.Info(fmt.Sprintf("Chosen message mechanism: %s", fc.Params.messaging))
	fc.UI.Info(fmt.Sprintf("Message rate (miliseconds): %v", fc.Params.messageRate))
	fc.UI.Info(fmt.Sprintf("Benchmark duration (seconds): %v", fc.Params.benchDuration))
	fc.UI.Info(fmt.Sprintf("Benchmark downtime duration (seconds): %v", fc.Params.benchDowntime))
	fc.UI.Info(fmt.Sprintf("Message size (bytes): %v", fc.Params.messageSize))
	fc.UI.Info(fmt.Sprintf("Peering degree: %v", fc.Params.peeringDegree))
	if fc.Params.nonValidatorDegree >= 0 {
		fc.UI.Info(fmt.Sprintf("Non-validator degree: %v", fc.Params.nonValidatorDegree))
	}
	if fc.Params.connectionCount >= 0 {
		fc.UI.Info(fmt.Sprintf("Connection count: %v", fc.Params.connectionCount))
	}

	fc.UI.Info("Starting benchmark...")

	StartGossipBench(ctx, fc.Params, topology, messaging, benchTimeout)

	fc.UI.Info("Benchmark executed")

	return 0
}

// NewFlagSet implements the interface and creates a new flag set for command arguments
func (fc *StartGossipCommand) NewFlagSet() *flag.FlagSet {
	flagSet := flag.NewFlagSet("libp2p-framework", flag.ContinueOnError)
	flagSet.IntVar(&fc.Params.nodeCount, "nodes", 10, "Count of nodes")
	flagSet.IntVar(&fc.Params.validatorCount, "validators", 2, "Count of validators")
	flagSet.StringVar(&fc.Params.topology, "topology", "linear", fmt.Sprintf("Topology of the nodes (%s, %s, %s)", linear, random, superCluster))
	flagSet.StringVar(&fc.Params.messaging, "messaging", "constant-rate", fmt.Sprintf("Messaging syncronization mechanism (%s, %s)", constantRate, hotStuff))
	flagSet.IntVar(&fc.Params.messageRate, "rate", 900, "Message rate (in milliseconds) of a node")
	flagSet.IntVar(&fc.Params.benchDuration, "duration", 40, "Duration of a benchmark in seconds")
	flagSet.IntVar(&fc.Params.benchDowntime, "downtime", 10, "Period of time in the end of benchmark for which logs will be discarded")
	flagSet.IntVar(&fc.Params.messageSize, "size", 1024, "Size (in bytes) of a transmitted message")
	flagSet.IntVar(&fc.Params.peeringDegree, "degree", 6, "Peering degree: count of directly connected peers")
	flagSet.IntVar(&fc.Params.nonValidatorDegree, "non-validator-degree", 6, "Peering degree: count of directly connected non-validator peers (super-cluster only)")
	flagSet.IntVar(&fc.Params.connectionCount, "connection-count", 1500, "Number of connections in random topology")
	flagSet.IntVar(&fc.Params.startingPort, "port", 10000, "Port of the first agent")

	return flagSet
}

func StartGossipBench(ctx context.Context, params GossipParameters, topology cluster.Topology, messaging cluster.Messaging, timeout time.Duration) {
	// remove file if exists
	// logger configuration
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{filepath.Join(outputFileDirectory, fmt.Sprintf("agents_%s.log", time.Now().Format(time.RFC3339)))}
	cfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:    "time",
		MessageKey: "msg",
	}
	cfg.Sampling = nil
	cfg.EncoderConfig.EncodeTime = SyslogTimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}

	logger.Info("Starting gossip benchmark",
		zap.Int("agentsCount", params.nodeCount),
		zap.Int("validatorsCount", params.validatorCount),
		zap.String("topology", fmt.Sprintf("%T", topology)),
		zap.String("messaging", fmt.Sprintf("%T", messaging)),
		zap.Int("benchDuration", params.benchDuration),
		zap.Int("msgRate", params.messageRate),
		zap.Int("peeringDegree", params.peeringDegree),
		zap.Int("nonValidatorDegree", params.nonValidatorDegree),
		zap.Int("connectionCount", params.connectionCount),
	)
	latencyData := lat.ReadLatencyDataFromJson()
	cluster := cluster.NewCluster(logger, latencyData, cluster.ClusterConfig{
		Ip:             IpString,
		StartingPort:   params.startingPort,
		MsgSize:        params.messageSize,
		ValidatorCount: params.validatorCount,
	})

	transportManager := network.NewTransportManager(createBaseConn, cluster.CreateConn)

	fmt.Println("Output file path: ", cfg.OutputPaths)
	fmt.Println("Start adding agents: ", params.nodeCount)

	// start agents in cluster
	acfg := agent.DefaultGossipConfig()
	acfg.Transport = transportManager.Transport()
	cluster.AddAgents(acfg, params.nodeCount)
	agentsStarted, agentsFailed, timeAdded := cluster.StartAgents()
	fmt.Printf("Agents added: %d. Failed agents: %v, Elapsed time: %v\n", agentsStarted, agentsFailed, timeAdded)
	cluster.ConnectAgents(topology)

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer func() {
		// flush logger buffer
		defer logger.Sync()
		// cleanup for the timeout context
		cancel()
	}()

	msgsPublishedCnt, msgsFailedCnt := cluster.StartMessaging(timeoutCtx, messaging)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
}

func SyslogTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.RFC3339))
}

func createBaseConn() (net.Conn, net.Conn) {
	return net.Pipe()
}

func GetGossipCommands() map[string]cli.CommandFactory {
	ui := &cli.BasicUi{
		Reader:      os.Stdin,
		Writer:      os.Stdout,
		ErrorWriter: os.Stderr,
	}
	return map[string]cli.CommandFactory{
		"start": func() (cli.Command, error) {
			return &StartGossipCommand{
				UI: ui,
			}, nil
		},
	}
}
