// udpconn implements Accept()-like semantics for a UDP listener
package udpconn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"syscall"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
)

var mcastIP = net.IP{224, 0, 0, 128}

// TODO: EPICS_PVA_BROADCAST_PORT environment variable
const udpPort = 5076

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

// Listener holds all the UDP sockets we're listening on.
// We need a bunch of sockets.
// One socket on INADDR_ANY with a random port to send beacons from
// For each interface,
//   Listen on addr:5076
//     IP_MULTICAST_IF 127.0.0.1
//     IP_MULTICAST_LOOP 1
//   Listen on broadcast:5076 (if interface has broadcast flag)
// One socket listening on 224.0.0.128 on lo
//   Listen on 224.0.0.128:5076
//   IP_ADD_MEMBERSHIP 224.0.0.128, 127.0.0.1
type Listener struct {
	sendConn               *net.UDPConn
	broadcastSendAddresses []*net.UDPAddr
	lns                    []*net.UDPConn
	connCh                 chan *Conn
}

func Listen(ctx context.Context) (*Listener, error) {
	sendConn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	ch := make(chan *Conn)
	ln := &Listener{
		sendConn: sendConn,
		lns:      []*net.UDPConn{sendConn},
		connCh:   ch,
	}
	if err := ln.bindInterfaces(ctx); err != nil {
		ln.Close()
		return nil, err
	}
	if err := ln.bindMulticast(ctx); err != nil {
		ln.Close()
		return nil, err
	}
	var ips []*net.UDPAddr
	for _, conn := range ln.lns {
		ips = append(ips, conn.LocalAddr().(*net.UDPAddr))
	}
	ctxlog.L(ctx).Infof("Listening on %v", ips)
	return ln, nil
}

func (ln *Listener) LocalAddr() *net.UDPAddr {
	return ln.sendConn.LocalAddr().(*net.UDPAddr)
}

func (ln *Listener) bindInterfaces(ctx context.Context) error {
	var ips []*net.UDPAddr
	var bcasts []*net.UDPAddr
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	for _, i := range interfaces {
		addrs, err := i.Addrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			if addr, ok := addr.(*net.IPNet); ok {
				laddr := &net.UDPAddr{
					IP:   addr.IP,
					Port: udpPort,
				}
				if addr.IP.To4() == nil {
					laddr.Zone = i.Name
					// TODO: Figure out what IPv6 multicast group we should use.
					continue
				}
				ips = append(ips, laddr)

				if err := ln.bindUnicast(ctx, laddr); err != nil {
					return err
				}

				if i.Flags&net.FlagBroadcast == net.FlagBroadcast {
					laddr := &net.UDPAddr{
						IP:   bcastIP(addr.IP, addr.Mask),
						Port: laddr.Port,
						Zone: laddr.Zone,
					}
					ips = append(ips, laddr)
					bcasts = append(bcasts, laddr)
					if err := ln.bindBroadcast(ctx, laddr); err != nil {
						return err
					}
				}
			}
		}
	}
	ln.broadcastSendAddresses = bcasts
	return nil
}

func bcastIP(ip net.IP, mask net.IPMask) net.IP {
	if len(mask) == net.IPv4len {
		ip = ip.To4()
	}
	var bcastip net.IP
	for i := range mask {
		bcastip = append(bcastip, ip[i]|(^mask[i]))
	}
	return bcastip
}

func (ln *Listener) bindUnicast(ctx context.Context, laddr *net.UDPAddr) error {
	udpConn, network, err := listen(ctx, "udp", laddr.String())
	if err != nil {
		return fmt.Errorf("listen %v: %v", laddr, err)
	}
	rawConn, err := udpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("can't obtain fd: %v", err)
	}
	var cerr error
	if err := rawConn.Control(func(fd uintptr) {
		if network == "udp6" {
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
		return err
	}
	if cerr != nil {
		return cerr
	}
	return ln.addConn(ctx, udpConn)
}

func (ln *Listener) bindBroadcast(ctx context.Context, laddr *net.UDPAddr) error {
	udpConn, _, err := listen(ctx, "udp", laddr.String())
	if err != nil {
		return fmt.Errorf("listen %v: %v", laddr, err)
	}
	return ln.addConn(ctx, udpConn)
}

func (ln *Listener) bindMulticast(ctx context.Context) error {
	laddr := &net.UDPAddr{
		IP:   mcastIP,
		Port: udpPort,
	}
	if runtime.GOOS == "windows" {
		laddr.IP = nil
	}
	udpConn, _, err := listen(ctx, "udp4", laddr.String())
	if err != nil {
		return fmt.Errorf("listen %v: %v", laddr, err)
	}
	rawConn, err := udpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("can't obtain fd: %v", err)
	}
	var cerr error
	if err := rawConn.Control(func(fd uintptr) {
		if err := syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, &syscall.IPMreq{
			Multiaddr: [4]byte{224, 0, 0, 128},
			Interface: [4]byte{127, 0, 0, 1},
		}); err != nil {
			cerr = err
		}
	}); err != nil {
		return err
	}
	if cerr != nil {
		return cerr
	}
	return ln.addConn(ctx, udpConn)
}

func (ln *Listener) addConn(ctx context.Context, udpConn *net.UDPConn) error {
	ln.lns = append(ln.lns, udpConn)
	go ln.readLoop(ctx, udpConn)
	return nil
}

func (ln *Listener) readLoop(ctx context.Context, conn *net.UDPConn) error {
	for {
		pkt := make([]byte, 1500)
		n, ua, err := conn.ReadFromUDP(pkt)
		if err != nil {
			return err
		}
		pkt = pkt[:n]
		ln.connCh <- &Conn{
			r:             bytes.NewReader(pkt),
			w:             ln.sendConn,
			sendAddresses: []*net.UDPAddr{ua},
			laddr:         conn.LocalAddr().(*net.UDPAddr),
		}
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

func (ln *Listener) BroadcastConn() *Conn {
	return &Conn{
		r:             &io.LimitedReader{N: 0},
		w:             ln.sendConn,
		sendAddresses: ln.broadcastSendAddresses,
		laddr:         ln.sendConn.LocalAddr().(*net.UDPAddr),
	}
}

func (ln *Listener) BroadcastSendAddresses() []*net.UDPAddr {
	return ln.broadcastSendAddresses
}

type Conn struct {
	r             io.Reader
	w             *net.UDPConn
	sendAddresses []*net.UDPAddr
	laddr         *net.UDPAddr
}

// Addr returns the remote address.
// Meaningless if called on a broadcast socket.
func (conn *Conn) Addr() *net.UDPAddr {
	return conn.sendAddresses[0]
}

func (conn *Conn) LocalAddr() *net.UDPAddr {
	return conn.laddr
}

func (conn *Conn) Read(p []byte) (int, error) {
	return conn.r.Read(p)
}

func (conn *Conn) Write(p []byte) (int, error) {
	for _, addr := range conn.sendAddresses {
		if _, err := conn.w.WriteToUDP(p, addr); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (conn *Conn) Close() error {
	return nil
	// Don't close conn.w since it's shared by multiple Conns
}

// SetSendAddress overrides the address that the connection sends to.
// This is used to handle CMD_ORIGIN_TAG.
func (conn *Conn) SetSendAddress(addr *net.UDPAddr) {
	conn.sendAddresses = []*net.UDPAddr{addr}
}
