package network

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"

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

	acceptCh chan connWithError
	isClosed int32
}

func (t *Transport) newConnection(conn net.Conn, laddr, raddr ma.Multiaddr) (upstream.Conn, error) {
	conn, err := t.manager.LatencyConnFactory.CreateConn(conn, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &manetConn{conn, laddr, raddr}, nil
}

func (t *Transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	if t.laddr == nil || raddr == nil {
		panic("laddr and raddr must be specified")
	}

	conn1, conn2 := t.manager.ConnManager.Get(t.laddr.String(), raddr.String())

	// listener side
	go func() {
		otherTransport := t.manager.GetTransport(p.Pretty())
		maconn, err := otherTransport.newConnection(conn2, raddr, t.laddr)
		if err != nil {
			panic(fmt.Sprintf("Error creating listener connection: %v, err: %v"+p.Pretty(), err))
		}
		resultConn, err := t.upgrader.Upgrade(context.Background(), t, maconn, network.DirInbound, t.peerId)
		if err != nil {
			panic(fmt.Sprintf("Error creating listener connection: %v, err: %v"+p.Pretty(), err))
		}
		otherTransport.acceptCh <- connWithError{conn: resultConn, err: err}
	}()

	// dialer side
	maconn, err := t.newConnection(conn1, t.laddr, raddr)
	if err != nil {
		return nil, err
	}
	return t.upgrader.Upgrade(ctx, t, maconn, network.DirOutbound, p)
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
	connWithError, hasMore := <-t.acceptCh
	if !hasMore {
		return nil, net.ErrClosed
	}
	if connWithError.err != nil {
		return nil, connWithError.err
	}
	return connWithError.conn, nil
}

func (t *Transport) Close() error {
	if atomic.CompareAndSwapInt32(&t.isClosed, 0, 1) {
		close(t.acceptCh)
	}
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

type connWithError struct {
	conn transport.CapableConn
	err  error
}
