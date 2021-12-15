package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/hashicorp/go-sockaddr/template"
	"github.com/maticnetwork/libp2p-gossip-bench/agent"
)

func main() {
	var bindAddr, proxyAddr, httpAddr, rendezvousString, city string

	flag.StringVar(&bindAddr, "bind-addr", "0.0.0.0:3000", "")
	flag.StringVar(&proxyAddr, "proxy-addr", "", "")
	flag.StringVar(&rendezvousString, "rendezvous", "meetme", "")
	flag.StringVar(&httpAddr, "http-addr", "", "")
	flag.StringVar(&city, "city", "", "")

	flag.Parse()

	var err error

	config := agent.DefaultConfig()
	config.City = city
	if config.Addr, err = getTCPAddr(bindAddr); err != nil {
		panic(err)
	}
	if httpAddr != "" {
		if config.HttpAddr, err = getTCPAddr(httpAddr); err != nil {
			panic(err)
		}
	}
	if proxyAddr != "" {
		if config.ProxyAddr, err = getTCPAddr(proxyAddr); err != nil {
			panic(err)
		}
	}

	config.RendezvousString = rendezvousString
	logger := log.New(os.Stdout, "", 0)

	a, err := agent.NewAgent(logger, config)
	if err != nil {
		panic(err)
	}
	handleSignals(a)
}

func getTCPAddr(raw string) (*net.TCPAddr, error) {
	addr, err := template.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	return &net.TCPAddr{IP: net.ParseIP(host), Port: port}, nil
}

func handleSignals(a *agent.Agent) {
	signalCh := make(chan os.Signal, 4)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	<-signalCh
	os.Exit(0)
}
