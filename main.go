package main

import (
	"context"
	"fmt"
	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	lat "github.com/maticnetwork/libp2p-gossip-bench/latency"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"github.com/maticnetwork/libp2p-gossip-bench/utils"
	"go.uber.org/zap/zapcore"
	"math/rand"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

const (
	AgentsNumber            = 400
	ValidatorsNumber        = 100
	StartingPort            = 10000
	MsgSize                 = 4096
	IpString                = "127.0.0.1"
	outputFileDirectory     = "/tmp"
	RandomConnectionsCount  = 2000
	RandomTopologyConnected = true
	MaxPeers                = 30
)

func main() {
	rand.Seed(time.Now().Unix())
	// remove file if exists
	// logger configuration
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{filepath.Join(outputFileDirectory, fmt.Sprintf("agents_%s.log", time.Now().Format(time.RFC3339)))}
	cfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:    "time",
		MessageKey: "msg",
	}
	cfg.EncoderConfig.EncodeTime = SyslogTimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	// flush buffer
	defer logger.Sync()

	connManager := network.NewConnManagerNetPipe()
	latencyData := lat.ReadLatencyDataFromJson()
	cluster := agent.NewCluster(logger, latencyData, agent.ClusterConfig{
		Ip:             IpString,
		StartingPort:   StartingPort,
		MsgSize:        MsgSize,
		ValidatorCount: ValidatorsNumber,
	})
	transportManager := network.NewTransportManager(connManager, cluster)

	fmt.Println("Start adding agents: ", AgentsNumber)

	// start agents in cluster
	acfg := agent.DefaultGossipConfig()
	acfg.Transport = transportManager.Transport()
	agentsAdded, agentsFailed, timeAdded := cluster.StartAgents(AgentsNumber, *acfg)
	fmt.Printf("Agents added: %d. Failed agents: %v, Elapsed time: %v\n", agentsAdded, agentsFailed, timeAdded)
	cluster.ConnectAgents(agent.LinearTopology{})
	// cluster.ConnectAgents(agent.RandomTopology{Count: RandomConnectionsCount, MaxPeers: MaxPeers, Connected: RandomTopologyConnected})
	//cluster.ConnectAgents(agent.SuperClusterTopology{ValidatorPeering: 9, NonValidatorPeering: 2})

	fmt.Println("Gossip started")
	msgsPublishedCnt, msgsFailedCnt := cluster.MessageLoop(context.Background(), time.Millisecond*900, time.Second*30)
	fmt.Printf("Published %d messages \n", msgsPublishedCnt)
	fmt.Printf("Failed %d messages \n", msgsFailedCnt)
}
func SyslogTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.RFC3339))
}
