package network

import (
	"sync"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	"github.com/libp2p/go-libp2p/config"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type Manager struct {
	lock         sync.Mutex
	transports   []*Transport
	QueryNetwork func(laddr, raddr ma.Multiaddr) Network
}

func (m *Manager) dial(laddr, raddr ma.Multiaddr) (manet.Conn, error) {
	if laddr == nil || raddr == nil {
		panic("bad")
	}

	network := m.QueryNetwork(laddr, raddr)

	rawConn, err := m.findRemote(raddr).listener.Dial(network)
	if err != nil {
		return nil, err
	}

	// outbound connection
	conn0 := &conn{rawConn, laddr, raddr}

	return conn0, nil
}

func (m *Manager) findRemote(raddr ma.Multiaddr) *Transport {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, t := range m.transports {
		if t.laddr.Equal(raddr) {
			return t
		}
	}

	panic("not found")
}

func (m *Manager) Transport() config.TptC {
	if m.transports == nil {
		m.transports = []*Transport{}
	}
	return func(h host.Host, u *tptu.Upgrader, cg connmgr.ConnectionGater) (transport.Transport, error) {
		tr := &Transport{
			Upgrader: u,
			Manager:  m,
			listener: Listen(1024 * 1024),
		}
		m.transports = append(m.transports, tr)
		return tr, nil
	}
}
