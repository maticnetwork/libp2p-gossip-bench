package agent

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

type Agent struct {
	host host.Host
}

type Config struct {
	Addr             *net.TCPAddr
	NatAddr          string
	RendezvousString string
}

func DefaultConfig() *Config {
	c := &Config{}
	return c
}

func NewAgent(logger *log.Logger, config *Config) (*Agent, error) {
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if config.NatAddr != "" {
			addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.NatAddr, config.Addr.Port))

			if addr != nil {
				addrs = []multiaddr.Multiaddr{addr}
			}
		}
		return addrs
	}

	host, err := libp2p.New(
		libp2p.ListenAddrs(listenAddr),
		libp2p.AddrsFactory(addrsFactory),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p stack: %v", err)
	}

	logger.Printf("Agent started: addr=%s", listenAddr.String())

	peerChan := initMDNS(host, config.RendezvousString)
	go func() {
		for {
			peer := <-peerChan
			logger.Printf("Found peer: peer=%s", peer)

			if err := host.Connect(context.Background(), peer); err != nil {
				fmt.Println("Connection failed:", err)
			}
		}
	}()

	a := &Agent{
		host: host,
	}
	return a, nil
}

func (a *Agent) Stop() {
	a.host.Close()
}

type discoveryNotifee struct {
	PeerChan chan peer.AddrInfo
}

//interface to be called when new  peer is found
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.PeerChan <- pi
}

//Initialize the MDNS service
func initMDNS(peerhost host.Host, rendezvous string) chan peer.AddrInfo {
	// register with service so that we get notified about peer discovery
	n := &discoveryNotifee{}
	n.PeerChan = make(chan peer.AddrInfo)

	// An hour might be a long long period in practical applications. But this is fine for us
	ser := mdns.NewMdnsService(peerhost, rendezvous, n)
	if err := ser.Start(); err != nil {
		panic(err)
	}
	return n.PeerChan
}
