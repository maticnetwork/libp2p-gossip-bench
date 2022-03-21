package latency

import (
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	"github.com/maticnetwork/libp2p-gossip-bench/proto"
	ma "github.com/multiformats/go-multiaddr"
)

type Mesh struct {
	lock sync.Mutex

	Latency  *LatencyData
	Logger   hclog.Logger
	Servers  map[string]*server
	Port     int
	Manager  *network.Manager
	MaxPeers int
}

func (m *Mesh) waitForPeers(min int64) {
	check := func() bool {
		for _, p := range m.Servers {
			if p.NumPeers() < min {
				return false
			}
		}
		return true
	}
	for {
		if check() {
			return
		}
	}
}

func (m *Mesh) findPeerByPort(port string) *server {
	for _, p := range m.Servers {
		if strconv.Itoa(p.config.Addr.Port) == port {
			return p
		}
	}
	panic("bad")
}

func (m *Mesh) QueryLatencies(laddr, raddr ma.Multiaddr) network.Network {
	m.lock.Lock()
	defer m.lock.Unlock()

	laddrPort, err := laddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		panic(err)
	}
	raddrPort, err := raddr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		panic(err)
	}

	laddrSrv := m.findPeerByPort(laddrPort)
	raddrSrv := m.findPeerByPort(raddrPort)

	latency := m.Latency.FindLatency(laddrSrv.City, raddrSrv.City)

	nn := network.Network{
		Kbps:    20 * 1024, // should I change this?
		Latency: latency,
		MTU:     1500,
	}
	return nn
}

func (m *Mesh) Gossip(from string, size int) {
	buf := make([]byte, size)
	rand.Read(buf)

	m.Servers[from].topic.Publish(&proto.Txn{
		From: from,
		Raw: &any.Any{
			Value: buf,
		},
	})
}

func (m *Mesh) Join(fromID, toID string) error {
	from := m.Servers[fromID]
	to := m.Servers[toID]
	return from.Join(to.AddrInfo(), 5*time.Second)
}

func (m *Mesh) RunServer(name string) error {
	if m.Servers == nil {
		m.Servers = map[string]*server{}
	}

	n := rand.Int() % len(m.Latency.SourcesList)
	city := m.Latency.SourcesList[n].Name

	config := network.DefaultConfig()
	config.Transport = m.Manager.Transport()
	config.City = city
	config.MaxPeers = uint64(m.MaxPeers)

	config.Addr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: m.Port}
	m.Port++

	m.Servers[name] = newServer(m.Logger.Named(name), config, city)
	return nil
}

func (m *Mesh) Stop() {
	for _, srv := range m.Servers {
		srv.Close()
	}
}
