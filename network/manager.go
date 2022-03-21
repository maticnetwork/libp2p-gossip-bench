package network

import (
	"net"

	"github.com/libp2p/go-libp2p-core/connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	"github.com/libp2p/go-libp2p/config"
	ma "github.com/multiformats/go-multiaddr"
)

type Manager struct {
	ConnManager         ConnManager
	NetworkQueryLatency NetworkQueryLatency
}

func NewManager(connMngr ConnManager, networkQueryLatency NetworkQueryLatency) *Manager {
	return &Manager{
		ConnManager:         connMngr,
		NetworkQueryLatency: networkQueryLatency,
	}
}

func (m *Manager) NewConnection(conn net.Conn, laddr, raddr ma.Multiaddr) (*manetConn, error) {
	conn, err := m.NetworkQueryLatency.CreateConn(conn, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &manetConn{conn, laddr, raddr}, nil
}

func (m *Manager) Transport() config.TptC {
	return func(h host.Host, u *tptu.Upgrader, cg connmgr.ConnectionGater) (transport.Transport, error) {
		tr := &Transport{
			Upgrader: u,
			Manager:  m,
		}
		return tr, nil
	}
}
