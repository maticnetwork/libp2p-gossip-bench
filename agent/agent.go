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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Shopify/toxiproxy/v2"
	"github.com/Shopify/toxiproxy/v2/toxics"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

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
	MaxPeers         int64
	MinInterval      int64
}

func DefaultConfig() *Config {
	c := &Config{
		//Addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 6000},
		//HttpAddr: &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 7000},
	}
	return c
}

// pubsubQueueSize is the size that we assign to our validation queue and outbound message queue for
// gossipsub.
const pubsubQueueSize = 600

func NewAgent(logger *log.Logger, config *Config) (*Agent, error) {
	listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", config.Addr.IP.String(), config.Addr.Port))
	if err != nil {
		return nil, err
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

	// start gossip protocol
	psOpts := []pubsub.Option{
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		//pubsub.WithNoAuthor(),
		pubsub.WithPeerOutboundQueueSize(pubsubQueueSize),
		pubsub.WithValidateQueueSize(pubsubQueueSize),
		pubsub.WithGossipSubParams(pubsubGossipParam()),
	}
	ps, err := pubsub.NewGossipSub(context.Background(), host, psOpts...)
	if err != nil {
		return nil, err
	}

	a := &Agent{
		logger:    logger,
		host:      host,
		gossipSub: ps,
		config:    config,
	}

	if config.HttpAddr != nil {
		a.setupHttp()
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

		logrus.SetFormatter(&logrus.JSONFormatter{})

		// Output to stdout instead of the default stderr
		// Can be any io.Writer, see below for File example
		logrus.SetOutput(os.Stdout)

		// Only log the warning severity or above.
		logrus.SetLevel(logrus.WarnLevel)

		proxy := toxiproxy.NewProxy()
		proxy.Name = "proxy"
		proxy.Listen = config.ProxyAddr.String()
		proxy.Upstream = config.Addr.String()

		sidecar := toxiproxy.NewServer()
		if err := sidecar.Collection.Add(proxy, true); err != nil {
			// In 5% of the cases not sure why but this port is already in use, then, remove the proxy
			// and use the normal connection.
			// I think this should work fine
			config.ProxyAddr = nil
			logger.Printf("[INFO]: Fallback proxy")
		} else {
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

						// fmt.Println(name)

						// ping that ip in name on the http port to get the name of the destination city

						// the ip we get from name is the bind-address which is 300x, the one from the http port is 400x
						ip := strings.Replace(name, ":30", ":40", -1)

						var latencyF int64

						dest, err := query("http://" + ip + "/system/city")
						if err != nil {
							logger.Printf("[INFO]: Add latency: name=%s, ip=%s default", name, ip)
							latencyF = time.Duration(100 * time.Millisecond).Milliseconds()
						} else {
							proxyLatency := latency.FindLatency(config.City, dest)

							logger.Printf("[INFO]: Add latency: name=%s, ip=%s, city=%s, latency=%s", name, ip, dest, proxyLatency)
							// logger.Printf("[INFO]: Add latency: ip=%s", name)

							latencyF = proxyLatency.Milliseconds()
						}
						//latency := time.Duration(300 * time.Millisecond).Milliseconds()

						link.AddToxic(&toxics.ToxicWrapper{
							Toxic: &toxics.LatencyToxic{
								Latency: latencyF,
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
	}

	logger.Printf("Agent started: addr=%s, city=%s", listenAddr.String(), config.City)

	if config.MaxPeers == 0 {
		panic("Max peers cannot be zero")
	}
	if config.MaxPeers > -1 {
		logger.Printf("Max peers: %d", config.MaxPeers)
	}

	peerChan := initMDNS(host, config.RendezvousString)
	go func() {
		for {
			peer := <-peerChan
			logger.Printf("[INFO] Found peer: peer=%s", peer)

			numPeers := len(host.Network().Peers())
			if config.MaxPeers != -1 {
				if numPeers > int(config.MaxPeers) {
					logger.Printf("[INFO]: Skip peer")
					continue
				}
			}

			if err := host.Connect(context.Background(), peer); err != nil {
				fmt.Println("Connection failed:", err)
			}
		}
	}()

	if a.topic, err = ps.Join("topic"); err != nil {
		return nil, err
	}
	sub, err := a.topic.Subscribe()
	if err != nil {
		return nil, err
	}
	readLoop(sub, func(msg *Msg) {
		elapsed := time.Since(msg.Time)

		logger.Printf("=> '%s' '%d' '%s' '%s' '%s' '%s'", elapsed, len(host.Network().Peers()), time.Now(), msg.From, msg.Hash, msg.Time)
	})

	if config.MinInterval != -1 {
		go func() {
			for {
				now := time.Now()
				if now.Minute()&int(config.MinInterval) == 0 {
					logger.Printf("[INFO]: Public at interval %d %d", config.MinInterval, now.Minute())
					go a.publish(100)
				}
				time.Sleep(1 * time.Minute)
			}
		}()
	}

	return a, nil
}

const (
	// overlay parameters
	gossipSubD   = 8  // topic stable mesh target count
	gossipSubDlo = 6  // topic stable mesh low watermark
	gossipSubDhi = 12 // topic stable mesh high watermark

	// gossip parameters
	gossipSubMcacheLen    = 6   // number of windows to retain full messages in cache for `IWANT` responses
	gossipSubMcacheGossip = 3   // number of windows to gossip about
	gossipSubSeenTTL      = 550 // number of heartbeat intervals to retain message IDs

	// fanout ttl
	gossipSubFanoutTTL = 60000000000 // TTL for fanout maps for topics we are not subscribed to but have published to, in nano seconds

	// heartbeat interval
	gossipSubHeartbeatInterval = 700 * time.Millisecond // frequency of heartbeat, milliseconds

	// misc
	randomSubD = 6 // random gossip target
)

// creates a custom gossipsub parameter set.
func pubsubGossipParam() pubsub.GossipSubParams {
	gParams := pubsub.DefaultGossipSubParams()
	gParams.Dlo = gossipSubDlo
	gParams.D = gossipSubD
	gParams.HeartbeatInterval = gossipSubHeartbeatInterval
	gParams.HistoryLength = gossipSubMcacheLen
	gParams.HistoryGossip = gossipSubMcacheGossip

	// Set a larger gossip history to ensure that slower
	// messages have a longer time to be propagated. This
	// comes with the tradeoff of larger memory usage and
	// size of the seen message cache.
	//if features.Get().EnableLargerGossipHistory {
	//	gParams.HistoryLength = 12
	//	gParams.HistoryGossip = 5
	//}
	return gParams
}

type Msg struct {
	From string
	Time time.Time
	Data string // Just some bulk random data
	Hash string
}

func query(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	sb := string(body)
	return sb, nil
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
	if err := a.topic.Publish(context.Background(), raw); err != nil {
		panic(err)
	}
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

		go a.publish(size)
		return c.JSON(http.StatusOK, map[string]interface{}{})
	})

	go func() {
		a.logger.Printf("Start http server: addr=%s", a.config.HttpAddr.String())
		if err := e.Start(a.config.HttpAddr.String()); err != nil {
			panic(err)
		}
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
