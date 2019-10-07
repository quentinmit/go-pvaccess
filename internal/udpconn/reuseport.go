package udpconn

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// TODO: Use syscall.SO_REUSEPORT on BSD, and nothing on Windows.

func listen(ctx context.Context, network, laddr string) (*net.UDPConn, string, error) {
	var networkRet string
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) (err error) {
			networkRet = network
			if cerr := c.Control(func(fd uintptr) {
				err = setSockOpts(int(fd))
			}); cerr != nil && err == nil {
				return cerr
			}
			return
		},
	}
	conn, err := lc.ListenPacket(ctx, network, laddr)
	if udpConn, ok := conn.(*net.UDPConn); ok {
		return udpConn, networkRet, err
	}
	return nil, networkRet, err
}

func setSockOpts(fd int) error {
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return err
	}
	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		return err
	}
	return syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}

// listenMulticast creates a socket bound to gaddr.
// It does NOT join the group.
// This exists to work around https://github.com/golang/go/issues/34728
func listenMulticast(ctx context.Context, gaddr *net.UDPAddr) (*net.UDPConn, func(), error) {
	if runtime.GOOS == "windows" {
		c, _, err := listen(ctx, "udp4", gaddr.String())
		return c, nil, err
	}
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return nil, nil, err
	}
	setSockOpts(s)
	ip := gaddr.IP.To4()
	if len(ip) != 4 {
		return nil, nil, errors.New("gaddr does not contain IPv4 address")
	}
	lsa := syscall.SockaddrInet4{
		Port: gaddr.Port,
	}
	copy(lsa.Addr[:], ip)
	if err := syscall.Bind(s, &lsa); err != nil {
		return nil, nil, err
	}
	f := os.NewFile(uintptr(s), "")
	c, err := net.FilePacketConn(f)
	if err != nil {
		return nil, nil, err
	}
	return c.(*net.UDPConn), func() { f.Close() }, err
}
