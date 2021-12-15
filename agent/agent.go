package agent

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Shopify/toxiproxy/v2"
	"github.com/Shopify/toxiproxy/v2/toxics"
	"github.com/labstack/echo/v4"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/multiformats/go-multiaddr"
)

//go:embed latencies.json
var latencyDataRaw string

type Agent struct {
	host      host.Host
	logger    *log.Logger
	gossipSub *pubsub.PubSub
	config    *Config
	topic     *pubsub.Topic
	sidecar   *toxiproxy.ApiServer
	latency   *LatencyData
}

type Config struct {
	Addr             *net.TCPAddr
	ProxyAddr        *net.TCPAddr
	HttpAddr         *net.TCPAddr
	RendezvousString string
	City             string
	ID               string
}

func DefaultConfig() *Config {
	c := &Config{
		Addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 6000},
		HttpAddr: &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 7000},
	}
	return c
}

func NewAgent(logger *log.Logger, config *Config) (*Agent, error) {
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
	}

	// read latency
	latency := readLatency()

	// try to decode the city
	if config.City == "" {
		logger.Printf("Generate new city...")
		config.City = latency.getRandomCity()
	}

	// start the proxy
	if config.ProxyAddr != nil {
		logger.Printf("[INFO]: Proxy/NAT %s", config.ProxyAddr.String())

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
			linkCh := make(chan *toxiproxy.ToxicLink, 1000)
			proxy.Toxics.LinkCh = linkCh

			for {
				link := <-linkCh

				go func(link *toxiproxy.ToxicLink) {

					name := link.Name()
					name = strings.TrimSuffix(name, "upstream")
					name = strings.TrimSuffix(name, "downstream")

					// ping that ip in name on the http port to get the name of the destination city
					ip := strings.Split(name, ":")[0]
					dest := query("http://" + ip + ":7000/system/city")
					proxyLatency := latency.FindLatency(config.City, dest)

					logger.Printf("[INFO]: Add latency: ip=%s, city=%s, latency=%s", name, dest, proxyLatency)

					// latency := 1 * time.Second

					link.AddToxic(&toxics.ToxicWrapper{
						Toxic: &toxics.LatencyToxic{
							Latency: proxyLatency.Milliseconds(),
						},
						Type:       "latency",
						Direction:  link.Direction(),
						BufferSize: 1024,
						Toxicity:   1,
					})
				}(link)
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

	logger.Printf("Agent started: addr=%s, city=%s", listenAddr.String(), config.City)

	// start gossip protocol
	ps, err := pubsub.NewGossipSub(context.Background(), host)
	if err != nil {
		return nil, err
	}

	peerChan := initMDNS(host, config.RendezvousString)
	go func() {
		for {
			peer := <-peerChan
			logger.Printf("[INFO] Found peer: peer=%s", peer)

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
	readLoop(sub, func(msg *Msg) {
		elapsed := time.Since(msg.Time)

		logger.Printf("=> '%s' '%s' '%s' '%s' '%s'", time.Now(), msg.From, msg.Hash, msg.Time, elapsed)
	})

	return a, nil
}

type Msg struct {
	From string
	Time time.Time
	Data string // Just some bulk random data
	Hash string
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

func (a *Agent) Stop() {
	a.host.Close()
}

func (a *Agent) publish(size int) {
	now := time.Now()

	data := make([]byte, size)
	rand.Read(data)

	hash := hashit(data)

	a.logger.Printf("Publish '%s' '%s'", hash, now)

	msg := &Msg{
		From: a.config.ID,
		Time: now,
		Data: string(data),
		Hash: hash,
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	a.topic.Publish(context.Background(), raw)
}

func hashit(b []byte) string {
	h := sha256.New()
	h.Write(b)
	dst := h.Sum(nil)
	return hex.EncodeToString(dst)
}

func (a *Agent) setupHttp() {
	e := echo.New()
	e.HideBanner = true
	e.Logger.SetOutput(ioutil.Discard)

	e.GET("/system/city", func(c echo.Context) error {
		return c.String(http.StatusOK, a.config.City)
	})
	e.GET("/system/id", func(c echo.Context) error {
		return c.String(http.StatusOK, a.config.ID)
	})
	e.GET("/publish", func(c echo.Context) error {

		size := 1024
		if sizeStr := c.QueryParam("size"); sizeStr != "" {
			s, err := strconv.Atoi(sizeStr)
			if err != nil {
				panic(err)
			}
			size = s
		}

		a.publish(size)
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

func readLoop(sub *pubsub.Subscription, handler func(msg *Msg)) {
	go func() {
		for {
			raw, err := sub.Next(context.Background())
			if err != nil {
				continue
			}
			var msg *Msg
			if err := json.Unmarshal(raw.Data, &msg); err != nil {
				panic(err)
			}
			handler(msg)
		}
	}()
}

type LatencyData struct {
	PingData map[string]map[string]struct {
		Avg string
	}
	SourcesList []struct {
		Id   string
		Name string
	}
	sources map[string]string
}

func (l *LatencyData) getRandomCity() string {
	n := rand.Int() % len(l.SourcesList)
	city := l.SourcesList[n].Name
	return city
}

func (l *LatencyData) checkCity(from string) bool {
	_, ok := l.sources[from]
	return ok
}

func (l *LatencyData) FindLatency(from, to string) time.Duration {
	fromID := l.sources[from]
	toID := l.sources[to]

	avg := l.PingData[fromID][toID].Avg
	if avg == "" {
		return 0
	}
	dur, err := time.ParseDuration(avg + "ms")
	if err != nil {
		fmt.Println(from, to, fromID, toID, avg)
		panic(err)
	}
	return dur
}

func readLatency() *LatencyData {
	var latencyData LatencyData
	if err := json.Unmarshal([]byte(latencyDataRaw), &latencyData); err != nil {
		panic(err)
	}

	latencyData.sources = map[string]string{}
	for _, i := range latencyData.SourcesList {
		latencyData.sources[i.Name] = i.Id
	}
	return &latencyData
}
