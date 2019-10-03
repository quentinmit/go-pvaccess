package udpconn

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

var reuseportListenConfig = net.ListenConfig{
	Control: reuseport,
}

// TODO: Use syscall.SO_REUSEPORT on BSD, and nothing on Windows.

func reuseport(network, address string, c syscall.RawConn) (err error) {
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
}
