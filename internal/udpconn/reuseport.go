package udpconn

import (
	"context"
	"net"
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
				if err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
					return
				}
				if err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
					return
				}
				if err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); err != nil {
					return
				}
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
