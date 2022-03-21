package network

import (
	"context"
	"net"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	ma "github.com/multiformats/go-multiaddr"
)

type Transport struct {
	// need the upgrader to create the connection
	Upgrader *tptu.Upgrader

	// reference to the transport latency manager
	Manager *Manager

	// listener is ready to accept connections
	listener *BuffconnListener

	// local address
	laddr ma.Multiaddr
}

func (t *Transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	conn, err := t.Manager.dial(t.laddr, raddr)
	if err != nil {
		return nil, err
	}
	return t.Upgrader.UpgradeOutbound(ctx, t, conn, p)
}

func (t *Transport) CanDial(addr ma.Multiaddr) bool {
	return true
}

func (t *Transport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	t.laddr = laddr

	lis := &listener{
		transport: t,
	}
	return lis, nil
}

func (t *Transport) Protocols() []int {
	// it is easier to override tcp than to figure out how to register a custom transport in multicodec
	return []int{ma.P_TCP}
}

func (t *Transport) Proxy() bool {
	return false
}

// -- listener --

type listener struct {
	transport *Transport
}

func (l *listener) Accept() (transport.CapableConn, error) {
	rawConn, err := l.transport.listener.Accept()
	if err != nil {
		return nil, err
	}
	manetConn := &conn{rawConn, l.transport.laddr, l.transport.laddr}
	return l.transport.Upgrader.UpgradeInbound(context.Background(), l.transport, manetConn)
}

func (l *listener) Close() error {
	return l.transport.listener.Close()
}

func (l *listener) Addr() net.Addr {
	panic("unimplemented")
	return nil
}

func (l *listener) Multiaddr() ma.Multiaddr {
	return l.transport.laddr
}

// -- connection --

type conn struct {
	net.Conn

	laddr ma.Multiaddr

	raddr ma.Multiaddr
}

func (c *conn) LocalMultiaddr() ma.Multiaddr {
	return c.laddr
}

func (c *conn) RemoteMultiaddr() ma.Multiaddr {
	return c.raddr
}
