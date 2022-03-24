package network

import (
	"net"
	"sync"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	"github.com/libp2p/go-libp2p/config"
)

type LatencyConnFactory interface {
	CreateConn(baseConn net.Conn, leftNodeId, rightNodeId int) (net.Conn, error)
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

func (m *TransportManager) GetTransport(peerId peer.ID) *Transport {
	m.lock.RLock()
	defer m.lock.RUnlock()
	transportId := peerId.Pretty()
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
			acceptCh: make(chan acceptChData),
		}
		m.transports[h.ID().Pretty()] = tr
		return tr, nil
	}
}
