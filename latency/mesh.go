package latency

import (
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"

	"github.com/maticnetwork/libp2p-gossip-bench/agent"
	"github.com/maticnetwork/libp2p-gossip-bench/network"
	ma "github.com/multiformats/go-multiaddr"
)

type Mesh struct {
	lock sync.Mutex

	Latency  *LatencyData
	Logger   *log.Logger
	Agents   map[string]*agent.Agent
	Port     int
	Manager  *network.Manager
	MaxPeers int
}

func (m *Mesh) findPeerByPort(port string) *agent.Agent {
	for _, a := range m.Agents {
		if strconv.Itoa(a.Config.Addr.Port) == port {
			return a
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

	latency := m.Latency.FindLatency(laddrSrv.Config.City, raddrSrv.Config.City)

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

	// TODO: Check if Agent topic Publish needs Proto
	// m.Agents[from].Topic.Publish(context.Background(), &proto.Txn{
	// 	From: from,
	// 	Raw: &any.Any{
	// 		Value: buf,
	// 	},
	// })
}

func (m *Mesh) Join(fromID, toID string) error {
	// from := m.Agents[fromID]
	// to := m.Agents[toID]

	// TODO: Implement Join for Agent
	return nil
}

func (m *Mesh) RunServer(name string) error {
	if m.Agents == nil {
		m.Agents = map[string]*agent.Agent{}
	}

	n := rand.Int() % len(m.Latency.SourcesList)
	city := m.Latency.SourcesList[n].Name

	config := agent.DefaultConfig()
	config.Transport = m.Manager.Transport()
	config.City = city
	config.MaxPeers = int64(m.MaxPeers)

	config.Addr = &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: m.Port}
	m.Port++

	agent, err := agent.NewAgent(m.Logger, config)
	if err != nil {
		return err
	}
	m.Agents[name] = agent
	return nil
}

func (m *Mesh) Stop() {
	for _, agnt := range m.Agents {
		agnt.Stop()
	}
}
