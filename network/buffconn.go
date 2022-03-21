/*
 *
 * Copyright 2017 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package bufconn provides a net.Conn implemented by a buffer and related
// dialing and listening functionality.
package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// BuffconnListener implements a net.BuffconnListener that creates local, buffered net.Conns
// via its Accept and Dial method.
type BuffconnListener struct {
	mu   sync.Mutex
	sz   int
	ch   chan net.Conn
	done chan struct{}
}

// Implementation of net.Error providing timeout
type netErrorTimeout struct {
	error
}

func (e netErrorTimeout) Timeout() bool   { return true }
func (e netErrorTimeout) Temporary() bool { return false }

var errClosed = fmt.Errorf("closed")
var errTimeout net.Error = netErrorTimeout{error: fmt.Errorf("i/o timeout")}

// Listen returns a Listener that can only be contacted by its own Dialers and
// creates buffered connections between the two.
func Listen(sz int) *BuffconnListener {
	return &BuffconnListener{sz: sz, ch: make(chan net.Conn), done: make(chan struct{})}
}

// Accept blocks until Dial is called, then returns a net.Conn for the server
// half of the connection.
func (l *BuffconnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, errClosed
	case c := <-l.ch:
		return c, nil
	}
}

// Close stops the listener.
func (l *BuffconnListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	select {
	case <-l.done:
		// Already closed.
		break
	default:
		close(l.done)
	}
	return nil
}

// Addr reports the address of the listener.
func (l *BuffconnListener) Addr() net.Addr { return addr{} }

// Dial creates an in-memory full-duplex network connection, unblocks Accept by
// providing it the server half of the connection, and returns the client half
// of the connection.
func (l *BuffconnListener) Dial(n Network) (net.Conn, error) {
	return l.DialContext(context.Background(), n)
}

type pipe2 struct {
	net.Conn
	pipe *pipe
}

func (p *pipe2) Close() error {
	panic("err")
}

// DialContext creates an in-memory full-duplex network connection, unblocks Accept by
// providing it the server half of the connection, and returns the client half
// of the connection.  If ctx is Done, returns ctx.Err()
func (l *BuffconnListener) DialContext(ctx context.Context, n Network) (net.Conn, error) {
	p1, p2 := newPipe(l.sz), newPipe(l.sz)

	doneCh := make(chan struct{}, 2)
	var conn1, conn2 net.Conn

	go func() {
		var err error
		if conn1, err = n.Conn(p1); err != nil {
			panic(err)
		}
		conn1 = &pipe2{conn1, p1}
		doneCh <- struct{}{}
	}()

	go func() {
		var err error
		if conn2, err = n.Conn(p2); err != nil {
			panic(err)
		}
		conn2 = &pipe2{conn2, p2}
		doneCh <- struct{}{}
	}()

	for i := 0; i < 2; i++ {
		<-doneCh
	}

	//fmt.Println(conn1)
	//fmt.Println(conn2)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.done:
		return nil, errClosed
	case l.ch <- &buffconn{conn1, conn2}:
		return &buffconn{conn2, conn1}, nil
	}
}

type pipe struct {
	mu sync.Mutex

	// buf contains the data in the pipe.  It is a ring buffer of fixed capacity,
	// with r and w pointing to the offset to read and write, respsectively.
	//
	// Data is read between [r, w) and written to [w, r), wrapping around the end
	// of the slice if necessary.
	//
	// The buffer is empty if r == len(buf), otherwise if r == w, it is full.
	//
	// w and r are always in the range [0, cap(buf)) and [0, len(buf)].
	buf  []byte
	w, r int

	wwait sync.Cond
	rwait sync.Cond

	// Indicate that a write/read timeout has occurred
	wtimedout bool
	rtimedout bool

	wtimer *time.Timer
	rtimer *time.Timer

	closed      bool
	writeClosed bool
}

func newPipe(sz int) *pipe {
	p := &pipe{buf: make([]byte, 0, sz)}
	p.wwait.L = &p.mu
	p.rwait.L = &p.mu

	p.wtimer = time.AfterFunc(0, func() {})
	p.rtimer = time.AfterFunc(0, func() {})
	return p
}

func (p *pipe) LocalAddr() net.Addr { return addr{} }

func (p *pipe) RemoteAddr() net.Addr { return addr{} }

func (p *pipe) SetDeadline(t time.Time) error { return nil }

func (p *pipe) SetReadDeadline(t time.Time) error { return nil }

func (p *pipe) SetWriteDeadline(t time.Time) error { return nil }

func (p *pipe) empty() bool {
	return p.r == len(p.buf)
}

func (p *pipe) full() bool {
	return p.r < len(p.buf) && p.r == p.w
}

func (p *pipe) Read(b []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Block until p has data.
	for {
		if p.closed {
			return 0, io.ErrClosedPipe
		}
		if !p.empty() {
			break
		}
		if p.writeClosed {
			return 0, io.EOF
		}
		if p.rtimedout {
			return 0, errTimeout
		}

		p.rwait.Wait()
	}
	wasFull := p.full()

	n = copy(b, p.buf[p.r:len(p.buf)])
	p.r += n
	if p.r == cap(p.buf) {
		p.r = 0
		p.buf = p.buf[:p.w]
	}

	// Signal a blocked writer, if any
	if wasFull {
		p.wwait.Signal()
	}

	return n, nil
}

func (p *pipe) Write(b []byte) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, io.ErrClosedPipe
	}
	for len(b) > 0 {
		// Block until p is not full.
		for {
			if p.closed || p.writeClosed {
				return 0, io.ErrClosedPipe
			}
			if !p.full() {
				break
			}
			if p.wtimedout {
				return 0, errTimeout
			}

			p.wwait.Wait()
		}
		wasEmpty := p.empty()

		end := cap(p.buf)
		if p.w < p.r {
			end = p.r
		}
		x := copy(p.buf[p.w:end], b)
		b = b[x:]
		n += x
		p.w += x
		if p.w > len(p.buf) {
			p.buf = p.buf[:p.w]
		}
		if p.w == cap(p.buf) {
			p.w = 0
		}

		// Signal a blocked reader, if any.
		if wasEmpty {
			p.rwait.Signal()
		}
	}
	return n, nil
}

func (p *pipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	// Signal all blocked readers and writers to return an error.
	p.rwait.Broadcast()
	p.wwait.Broadcast()
	return nil
}

func (p *pipe) closeWrite() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeClosed = true
	// Signal all blocked readers and writers to return an error.
	p.rwait.Broadcast()
	p.wwait.Broadcast()
	return nil
}

type buffconn struct {
	io.Reader
	io.Writer
}

func (c *buffconn) Close() error {
	err1 := c.Reader.(*pipe2).pipe.Close()
	err2 := c.Writer.(*pipe2).pipe.closeWrite()
	if err1 != nil {
		return err1
	}
	return err2
}

func (c *buffconn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

func (c *buffconn) SetReadDeadline(t time.Time) error {
	return nil

	p := c.Reader.(*pipe)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rtimer.Stop()
	p.rtimedout = false
	if !t.IsZero() {
		p.rtimer = time.AfterFunc(time.Until(t), func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.rtimedout = true
			p.rwait.Broadcast()
		})
	}
	return nil
}

func (c *buffconn) SetWriteDeadline(t time.Time) error {
	return nil

	p := c.Writer.(*pipe)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.wtimer.Stop()
	p.wtimedout = false
	if !t.IsZero() {
		p.wtimer = time.AfterFunc(time.Until(t), func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.wtimedout = true
			p.wwait.Broadcast()
		})
	}
	return nil
}

func (*buffconn) LocalAddr() net.Addr  { return addr{} }
func (*buffconn) RemoteAddr() net.Addr { return addr{} }

type addr struct{}

func (addr) Network() string { return "bufconn" }
func (addr) String() string  { return "bufconn" }
