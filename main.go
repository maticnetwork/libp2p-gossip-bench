package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
)

func main() {

	var bindAddr, natAddr, rendezvousString string
	var bindPort uint64

	flag.StringVar(&bindAddr, "bind-addr", "0.0.0.0", "")
	flag.Uint64Var(&bindPort, "bind-port", 3000, "")
	flag.StringVar(&natAddr, "nat", "", "")
	flag.StringVar(&rendezvousString, "rendezvous", "meetme", "")

	flag.Parse()

	config := agent.DefaultConfig()
	config.NatAddr = natAddr
	config.Addr = &net.TCPAddr{IP: net.ParseIP(bindAddr), Port: int(bindPort)}
	config.RendezvousString = rendezvousString
	logger := log.New(os.Stdout, "", 0)

	a, err := agent.NewAgent(logger, config)
	if err != nil {
		panic(err)
	}
	handleSignals(a)
}

func handleSignals(a *agent.Agent) {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-signalCh
	os.Exit(0)
}
