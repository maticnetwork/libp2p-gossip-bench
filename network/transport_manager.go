package network

import (
	"net"
	"sync"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	"github.com/libp2p/go-libp2p/config"
	ma "github.com/multiformats/go-multiaddr"
)

type LatencyConnFactory interface {
	CreateConn(baseConn net.Conn, laddr, raddr ma.Multiaddr) (net.Conn, error)
}

type TransportManager struct {
	lock               sync.RWMutex
	transports         map[string]*Transport
	ConnManager        ConnManager
	LatencyConnFactory LatencyConnFactory
}

func NewTransportManager(connMngr ConnManager, latencyConnFactory LatencyConnFactory) *TransportManager {
	return &TransportManager{
		ConnManager:        connMngr,
		LatencyConnFactory: latencyConnFactory,
		transports:         make(map[string]*Transport),
		lock:               sync.RWMutex{},
	}
}

func (m *TransportManager) GetTransport(transportId string) *Transport {
	m.lock.RLock()
	defer m.lock.RUnlock()
	t, exists := m.transports[transportId]
	if !exists {
		panic("Transport does not exists: " + transportId)
	}
	return t
}

func (m *TransportManager) Transport() config.TptC {
	return func(h host.Host, u *tptu.Upgrader, cg connmgr.ConnectionGater) (transport.Transport, error) {
		m.lock.Lock()
		defer m.lock.Unlock()
		tr := &Transport{
			upgrader: u,
			manager:  m,
			peerId:   h.ID(),
		}
		m.transports[h.ID().Pretty()] = tr
		return tr, nil
	}
}
