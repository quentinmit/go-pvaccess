// udpconn implements Accept()-like semantics for a UDP listener
package udpconn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
)

func ipv6LoopbackIndex(ctx context.Context) int {
	interfaces, err := net.Interfaces()
	if err != nil {
		return -1
	}
	for _, intf := range interfaces {
		if addrs, err := intf.Addrs(); err == nil {
			for _, addr := range addrs {
				if addr, ok := addr.(*net.IPNet); ok && addr.IP.Equal(net.IPv6loopback) {
					return intf.Index
				}
			}
		}
	}
	ctxlog.L(ctx).Warn("couldn't find loopback interface")
	return -1
}

type Listener struct {
	lns    []*net.UDPConn
	connCh chan *Conn
	mu     sync.Mutex
	conns  map[addr]*Conn
}

func Listen(ctx context.Context, network string, laddr *net.UDPAddr) (*Listener, error) {
	conn, err := reuseportListenConfig.ListenPacket(ctx, network, laddr.String())
	if err != nil {
		return nil, fmt.Errorf("listen %v: %v", laddr, err)
	}
	udpConn := conn.(*net.UDPConn)
	ch := make(chan *Conn)
	ln := &Listener{
		lns:    []*net.UDPConn{udpConn},
		conns:  make(map[addr]*Conn),
		connCh: ch,
	}

	rawConn, err := udpConn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("can't obtain fd: %v", err)
	}
	var cerr error
	if err := rawConn.Control(func(fd uintptr) {
		// TODO: Set IPv6 sockopts if the socket is IPv6.
		ctxlog.L(ctx).Debugf("network = %q", udpConn.LocalAddr().Network())
		if udpConn.LocalAddr().Network() == "udp6" {
			if loIntf := ipv6LoopbackIndex(ctx); loIntf >= 0 {
				if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_IF, loIntf); err != nil {
					cerr = err
				}
			}
			if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_LOOP, 1); err != nil {
				cerr = err
			}
		} else {
			a := [4]byte{127, 0, 0, 1}
			if err := syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, a); err != nil {
				cerr = err
			}
			if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, 1); err != nil {
				cerr = err
			}
		}
	}); err != nil {
		return nil, err
	}
	if cerr != nil {
		return nil, cerr
	}

	go ln.readLoop(udpConn)
	return ln, nil
}

func (ln *Listener) AddReadAddress(ctx context.Context, network string, laddr *net.UDPAddr) error {
	conn, err := reuseportListenConfig.ListenPacket(ctx, network, laddr.String())
	if err != nil {
		return fmt.Errorf("listen %v: %v", laddr, err)
	}
	udpConn := conn.(*net.UDPConn)
	ln.lns = append(ln.lns, udpConn)
	go ln.readLoop(udpConn)
	return nil
}

func (ln *Listener) readLoop(conn *net.UDPConn) error {
	for {
		pkt := make([]byte, 1500)
		n, ua, err := conn.ReadFromUDP(pkt)
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
			ln.conns[key] = ln.newConn(key, ua)
			ln.connCh <- ln.conns[key]
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

func (ln *Listener) Close() (err error) {
	close(ln.connCh)
	for _, conn := range ln.lns {
		if cerr := conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return
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

func (ln *Listener) newConn(key addr, addr *net.UDPAddr) *Conn {
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
	// Always write from the first Listener
	return conn.ln.lns[0].WriteToUDP(p, conn.addr)
}

func (conn *Conn) Close() error {
	conn.packetIn.Close()
	return conn.ln.closeAddr(conn.key)
}

type MultiConn struct {
	SendAddresses []*net.UDPAddr
	conn          *net.UDPConn
}

func NewMulti() (*MultiConn, error) {
	ln, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	return &MultiConn{conn: ln}, nil
}

func (mc *MultiConn) Read(p []byte) (int, error) {
	return mc.Read(p)
}

func (mc *MultiConn) Write(p []byte) (int, error) {
	for _, addr := range mc.SendAddresses {
		if _, err := mc.conn.WriteToUDP(p, addr); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (mc *MultiConn) Close() error {
	return mc.conn.Close()
}
