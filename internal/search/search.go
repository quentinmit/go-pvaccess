// search handles beacons, search requests and responses
package search

import (
	"context"
	"crypto/rand"
	"net"
	"time"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/proto"
)

type server struct {
	lastBeacon proto.BeaconMessage
}

const udpPort = 5076

const beaconInterval = 5 * time.Second

func Serve(ctx context.Context, serverAddr *net.TCPAddr) error {
	var beacon proto.BeaconMessage
	if _, err := rand.Read(beacon.GUID[:]); err != nil {
		return err
	}
	ips := []net.IP{serverAddr.IP}
	if serverAddr.IP == nil || serverAddr.IP.IsUnspecified() {
		ips = nil
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			if addr, ok := addr.(*net.IPNet); ok {
				ips = append(ips, addr.IP)
			}
		}
	}
	beacon.ServerPort = uint16(serverAddr.Port)
	beacon.Protocol = "tcp"

	var senders []*sender
	for _, ip := range ips {
		var raddr = &net.UDPAddr{
			IP:   net.IPv6linklocalallnodes,
			Port: udpPort,
		}
		if ip4 := ip.To4(); len(ip4) == net.IPv4len {
			raddr.IP = net.IPv4bcast
		}
		conn, err := net.DialUDP("udp", &net.UDPAddr{IP: ip}, raddr)
		if err != nil {
			return err
		}
		senders = append(senders, &sender{
			conn: connection.New(conn, proto.FLAG_FROM_SERVER),
			ip:   ip,
		})
	}

	ticker := time.NewTicker(beaconInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			beacon.BeaconSequenceID++
			for _, s := range senders {
				s.send(ctx, beacon)
			}
		}
	}
}

type sender struct {
	conn *connection.Connection
	ip   net.IP
}

func (s sender) send(ctx context.Context, pkt proto.BeaconMessage) error {
	copy(pkt.ServerAddress[:], s.ip)
	return s.conn.SendApp(ctx, proto.APP_BEACON, &pkt)
}
