package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"

	"github.com/hashicorp/go-sockaddr/template"
	"github.com/maticnetwork/libp2p-gossip-bench/agent"

	"github.com/docker/docker/pkg/stdcopy"
)

func main() {
	cmd := os.Args[1]
	os.Args = os.Args[1:] // if we dont do this the flags are not parsed correctly

	if cmd == "server" {
		serverCmd()
	} else if cmd == "publish" {
		publishCmd()
	} else if cmd == "gather" {
		gatherCmd()
	} else {
		panic("NOT FOUND " + cmd)
	}
}

func gatherCmd() {
	var output string

	flag.StringVar(&output, "output", "", "")
	flag.Parse()

	output = "test-" + output
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		panic("folder exists")
	}

	if err := os.Mkdir(output, 0755); err != nil {
		panic(err)
	}

	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	containers := getGossipContainers()
	for _, c := range containers {
		fmt.Println(c)

		ip := getIP(c)
		url := "http://" + ip + ":7000/system/id"
		hostname := query(url)
		fmt.Println(hostname)

		out, err := cli.ContainerLogs(ctx, c.ID, types.ContainerLogsOptions{ShowStdout: true})
		if err != nil {
			panic(err)
		}

		buf := bytes.NewBuffer([]byte{})
		if _, err := stdcopy.StdCopy(buf, os.Stderr, out); err != nil {
			panic(err)
		}

		if err := os.WriteFile(filepath.Join(output, hostname), buf.Bytes(), 0644); err != nil {
			panic(err)
		}
	}
}

func getGossipContainers() []types.Container {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		panic(err)
	}

	gossipContainers := []types.Container{}
	for _, container := range containers {
		if container.Labels["gossip"] == "true" {
			gossipContainers = append(gossipContainers, container)
		}
	}
	return gossipContainers
}

func getIP(container types.Container) string {
	return container.NetworkSettings.Networks["bridge"].IPAddress
}

func publishCmd() {
	var numPublishers uint64
	var numMessages uint64
	var size uint64

	flag.Uint64Var(&numPublishers, "num-publishers", 0, "")
	flag.Uint64Var(&numMessages, "num-messages", 1, "")
	flag.Uint64Var(&size, "size", 100, "")
	flag.Parse()

	gossipContainers := getGossipContainers()

	if numPublishers == 0 {
		panic("no publishers in args")
	}
	if int(numPublishers) > len(gossipContainers) {
		panic(fmt.Sprintf("more num publishers than available containers %d %d", numPublishers, len(gossipContainers)))
	}

	var wg sync.WaitGroup
	for i := 0; i < int(numPublishers); i++ {
		wg.Add(1)

		ip := getIP(gossipContainers[i])
		go func(ip string) {
			defer wg.Done()

			for j := uint64(0); j < numMessages; j++ {
				url := "http://" + ip + ":7000/publish?size=" + strconv.Itoa(int(size))
				fmt.Println(url)
				fmt.Println(query(url))
			}
		}(ip)
	}

	wg.Wait()
}

func query(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	sb := string(body)
	return sb
}

func serverCmd() {
	var bindAddr, proxyAddr, httpAddr, rendezvousString, city string
	var maxPeers int64

	flag.StringVar(&bindAddr, "bind-addr", "0.0.0.0:3000", "")
	flag.StringVar(&proxyAddr, "proxy-addr", "", "")
	flag.StringVar(&rendezvousString, "rendezvous", "meetme", "")
	flag.StringVar(&httpAddr, "http-addr", "", "")
	flag.StringVar(&city, "city", "", "")
	flag.Int64Var(&maxPeers, "max-peers", -1, "")

	flag.Parse()

	var err error

	config := agent.DefaultConfig()
	config.MaxPeers = maxPeers

	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	config.ID = hostname

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
