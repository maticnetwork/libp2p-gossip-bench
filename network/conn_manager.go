package network

import (
	"net"
	"sync"
)

type ConnManagerFactory func() (net.Conn, net.Conn)

type ConnManager interface {
	Add(laddr, raddr string) (net.Conn, net.Conn)
	Get(laddr, raddr string) (net.Conn, net.Conn)
}

type ConnManagerImpl struct {
	lock        sync.RWMutex
	connFactory ConnManagerFactory
	connections map[string]map[string]net.Conn
}

func NewConnManagerImpl(connFactory ConnManagerFactory) *ConnManagerImpl {
	return &ConnManagerImpl{
		lock:        sync.RWMutex{},
		connFactory: connFactory,
		connections: make(map[string]map[string]net.Conn),
	}
}

func NewConnManagerNetPipe() *ConnManagerImpl {
	return NewConnManagerImpl(func() (net.Conn, net.Conn) {
		return net.Pipe()
	})
}

func (m *ConnManagerImpl) Add(laddr, raddr string) (net.Conn, net.Conn) {
	m.lock.Lock()
	defer m.lock.Unlock()
	connDialer, connListener := m.connFactory()
	return m.addInLock(laddr, raddr, connDialer), m.addInLock(raddr, laddr, connListener)
}

func (m *ConnManagerImpl) Get(laddr, raddr string) (net.Conn, net.Conn) {
	m.lock.RLock()
	var conn1, conn2 net.Conn = nil, nil
	if m.connections[laddr] != nil {
		conn1 = m.connections[laddr][raddr]
	}
	if m.connections[raddr] != nil {
		conn2 = m.connections[raddr][laddr]
	}
	m.lock.RUnlock()
	if conn1 == nil || conn2 == nil {
		conn1, conn2 = m.Add(laddr, raddr)
	}
	return conn1, conn2
}

func (m *ConnManagerImpl) addInLock(local, remote string, conn net.Conn) net.Conn {
	if m.connections[local] == nil {
		m.connections[local] = make(map[string]net.Conn)
	}
	m.connections[local][remote] = conn
	return conn
}
