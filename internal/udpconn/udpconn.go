// udpconn implements Accept()-like semantics for a UDP listener
package udpconn

import (
	"errors"
	"io"
	"net"
	"sync"
)

type Listener struct {
	conn   *net.UDPConn
	connCh <-chan *Conn
	mu     sync.Mutex
	conns  map[addr]*Conn
}

func Listen(network string, laddr *net.UDPAddr) (*Listener, error) {
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	ch := make(chan *Conn)
	ln := &Listener{
		conn:   conn,
		conns:  make(map[addr]*Conn),
		connCh: ch,
	}
	go ln.readLoop(ch)
	return ln, nil
}

func (ln *Listener) readLoop(connCh chan<- *Conn) error {
	defer close(connCh)
	for {
		pkt := make([]byte, 1500)
		n, ua, err := ln.conn.ReadFromUDP(pkt)
		if err != nil {
			return err
		}
		pkt = pkt[:n]
		key := addr{
			Port: ua.Port,
			Zone: ua.Zone,
		}
		copy(key.IP[:], ua.IP)
		ln.mu.Lock()
		if _, ok := ln.conns[key]; !ok {
			ln.conns[key] = newConn(ln, key, ua)
			connCh <- ln.conns[key]
		}
		conn := ln.conns[key]
		ln.mu.Unlock()
		conn.packetIn.Write(pkt)
	}
}

func (ln *Listener) Accept() (*Conn, error) {
	conn, ok := <-ln.connCh
	if !ok {
		return nil, errors.New("listener is closed")
	}
	return conn, nil
}

func (ln *Listener) Close() error {
	return ln.conn.Close()
}

func (ln *Listener) closeAddr(key addr) error {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	delete(ln.conns, key)
	return nil
}

type addr struct {
	IP   [16]byte
	Port int
	Zone string
}

type Conn struct {
	addr     *net.UDPAddr
	key      addr
	packetIn io.WriteCloser

	r  io.ReadCloser
	ln *Listener
}

func newConn(ln *Listener, key addr, addr *net.UDPAddr) *Conn {
	conn := &Conn{
		addr: addr,
		key:  key,
		ln:   ln,
	}
	r, w := io.Pipe()
	conn.packetIn = w
	conn.r = r
	return conn
}

func (conn *Conn) Addr() *net.UDPAddr {
	return conn.addr
}

func (conn *Conn) Read(p []byte) (int, error) {
	return conn.r.Read(p)
}

func (conn *Conn) Write(p []byte) (int, error) {
	return conn.ln.conn.WriteToUDP(p, conn.addr)
}

func (conn *Conn) Close() error {
	conn.packetIn.Close()
	return conn.ln.closeAddr(conn.key)
}
