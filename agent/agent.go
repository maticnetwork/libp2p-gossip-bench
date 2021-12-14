package agent

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"

	"github.com/Shopify/toxiproxy/v2"
	"github.com/Shopify/toxiproxy/v2/toxics"
	"github.com/labstack/echo/v4"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

type Agent struct {
	host      host.Host
	logger    *log.Logger
	gossipSub *pubsub.PubSub
	config    *Config
	topic     *pubsub.Topic
	sidecar   *toxiproxy.ApiServer
}

type Config struct {
	Addr             *net.TCPAddr
	ProxyAddr        *net.TCPAddr
	HttpAddr         *net.TCPAddr
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

	// start the proxy
	if config.ProxyAddr != nil {
		proxy := toxiproxy.NewProxy()
		proxy.Name = "proxy"
		proxy.Listen = config.ProxyAddr.String()
		proxy.Upstream = config.Addr.String()

		sidecar := toxiproxy.NewServer()
		if err := sidecar.Collection.Add(proxy, true); err != nil {
			panic(err)
		}

		go func() {
			// pick up every link created in the proxy and add the specific latency toxic
			linkCh := make(chan *toxiproxy.ToxicLink)
			proxy.Toxics.LinkCh = linkCh

			for {
				link := <-linkCh
				fmt.Println(link)

				link.AddToxic(&toxics.ToxicWrapper{
					Name: "XX",
				})
			}
		}()
	}

	addrsFactory := func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if config.ProxyAddr != nil {
			addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.ProxyAddr.IP.String(), config.ProxyAddr.Port))

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

	// start gossip protocol
	ps, err := pubsub.NewGossipSub(context.Background(), host)
	if err != nil {
		return nil, err
	}

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
		logger:    logger,
		host:      host,
		gossipSub: ps,
		config:    config,
	}

	if config.HttpAddr != nil {
		a.setupHttp()
	}

	if a.topic, err = ps.Join("topic"); err != nil {
		return nil, err
	}
	sub, err := a.topic.Subscribe()
	if err != nil {
		return nil, err
	}
	readLoop(sub, func(data []byte) {
		fmt.Println(data)
	})

	return a, nil
}

func (a *Agent) Stop() {
	a.host.Close()
}

func (a *Agent) publish() {
	data := make([]byte, 1024)
	rand.Read(data)

	a.topic.Publish(context.Background(), data)
}

func (a *Agent) setupHttp() {
	e := echo.New()
	e.HideBanner = true
	e.Logger.SetOutput(ioutil.Discard)

	e.GET("/", func(c echo.Context) error {
		a.publish()
		return c.JSON(http.StatusOK, map[string]interface{}{})
	})

	go func() {
		a.logger.Printf("Start http server: addr=%s", a.config.HttpAddr.String())
		e.Logger.Fatal(e.Start(a.config.HttpAddr.String()))
	}()
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
	ser := NewMdnsService(peerhost, rendezvous, n)
	if err := ser.Start(); err != nil {
		panic(err)
	}
	return n.PeerChan
}

func readLoop(sub *pubsub.Subscription, handler func(obj []byte)) {
	go func() {
		for {
			msg, err := sub.Next(context.Background())
			if err != nil {
				continue
			}
			handler(msg.Data)
		}
	}()
}
