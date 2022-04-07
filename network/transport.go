package network

import (
	"context"
	"net"
	"strconv"
	"sync"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/transport"
	tptu "github.com/libp2p/go-libp2p-transport-upgrader"
	ma "github.com/multiformats/go-multiaddr"
	upstream "github.com/multiformats/go-multiaddr/net"
)

type Transport struct {
	// need the upgrader to create the connection
	upgrader *tptu.Upgrader

	// reference to the transport latency manager
	manager *TransportManager

	// local address
	laddr ma.Multiaddr

	peerId peer.ID

	closedCh chan struct{}
	acceptCh chan acceptChData
	once     sync.Once // Protects closing closedCh
}

func (t *Transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	if t.laddr == nil || raddr == nil {
		panic("laddr and raddr must be specified")
	}

	conn1, conn2 := t.manager.BaseConnFactory()

	// listener side
	go func() {
		other := t.manager.GetTransport(p)
		other.acceptCh <- acceptChData{conn2, t.peerId, t.laddr}
	}()

	// dialer side
	latencyConn, err := t.manager.LatencyConnFactory(conn1, getPort(t.laddr), getPort(raddr))
	if err != nil {
		return nil, err
	}
	return t.upgrader.Upgrade(ctx, t, &manetConn{latencyConn, t.laddr, raddr}, network.DirOutbound, p)
}

func (t *Transport) CanDial(addr ma.Multiaddr) bool {
	return true
}

func (t *Transport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	t.laddr = laddr
	return t, nil
}

func (t *Transport) Protocols() []int {
	// it is easier to override tcp than to figure out how to register a custom transport in multicodec
	return []int{ma.P_TCP}
}

func (t *Transport) Proxy() bool {
	return false
}

func (t *Transport) Accept() (transport.CapableConn, error) {
	var data acceptChData
	select {
	case data = <-t.acceptCh:
	case <-t.closedCh:
		return nil, net.ErrClosed
	}

	latencyConn, err := t.manager.LatencyConnFactory(data.conn, getPort(t.laddr), getPort(data.raddr))
	if err != nil {
		return nil, err
	}
	return t.upgrader.Upgrade(context.Background(), t, &manetConn{latencyConn, t.laddr, data.raddr}, network.DirInbound, data.peerId)
}

func (t *Transport) Close() error {
	t.once.Do(func() {
		close(t.closedCh)
	})
	return nil
}

func (t *Transport) Addr() net.Addr {
	v, _ := upstream.ToNetAddr(t.laddr)
	return v
}

func (t *Transport) Multiaddr() ma.Multiaddr {
	return t.laddr
}

// -- multi address connection --

type manetConn struct {
	net.Conn

	laddr ma.Multiaddr

	raddr ma.Multiaddr
}

func (c *manetConn) LocalMultiaddr() ma.Multiaddr {
	return c.laddr
}

func (c *manetConn) RemoteMultiaddr() ma.Multiaddr {
	return c.raddr
}

type acceptChData struct {
	conn   net.Conn
	peerId peer.ID
	raddr  ma.Multiaddr
}

func getPort(addr ma.Multiaddr) int {
	sport, err := addr.ValueForProtocol(ma.P_TCP)
	if err != nil {
		panic("invalid port for " + addr.String())
	}
	r, _ := strconv.Atoi(sport)
	return r
}
