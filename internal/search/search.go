// search handles beacons, search requests and responses
package search

import (
	"context"
	"crypto/rand"
	"io"
	"net"
	"time"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/internal/udpconn"
)

type server struct {
	lastBeacon proto.BeaconMessage
}

const udpPort = 5076

const startupInterval = time.Second
const startupCount = 15
const beaconInterval = 5 * time.Second

func Serve(ctx context.Context, serverAddr *net.TCPAddr) error {
	var beacon proto.BeaconMessage
	if _, err := rand.Read(beacon.GUID[:]); err != nil {
		return err
	}
	ips := []*net.IPAddr{{IP: serverAddr.IP, Zone: serverAddr.Zone}}
	if serverAddr.IP == nil || serverAddr.IP.IsUnspecified() {
		ips = nil
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
					ips = append(ips, &net.IPAddr{IP: addr.IP, Zone: i.Name})
				}
			}
		}
	}
	ctxlog.L(ctx).Infof("sending beacons on %v", ips)
	beacon.ServerPort = uint16(serverAddr.Port)
	beacon.Protocol = "tcp"

	var senders []*sender
	for _, ip := range ips {
		var raddr = &net.UDPAddr{
			IP:   net.IPv6linklocalallnodes,
			Port: udpPort,
		}
		if ip4 := ip.IP.To4(); len(ip4) == net.IPv4len {
			raddr.IP = net.IPv4bcast
		}
		conn, err := net.DialUDP("udp", &net.UDPAddr{IP: ip.IP, Zone: ip.Zone}, raddr)
		if err != nil {
			return err
		}
		senders = append(senders, &sender{
			conn: connection.New(conn, proto.FLAG_FROM_SERVER),
			ip:   ip.IP,
		})
		/*
			go (&searchServer{
				GUID: beacon.GUID,
			}).serve(ctx, &net.UDPAddr{IP: ip.IP, Port: udpPort, Zone: ip.Zone})
		*/
	}

	go (&searchServer{
		GUID: beacon.GUID,
	}).serve(ctx, &net.UDPAddr{Port: udpPort})

	ticker := time.NewTicker(startupInterval)
	defer func() { ticker.Stop() }()
	i := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			beacon.BeaconSequenceID++
			for _, s := range senders {
				s.send(ctx, beacon)
			}
			i++
			if i == startupCount {
				ticker.Stop()
				ticker = time.NewTicker(beaconInterval)
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

type searchServer struct {
	GUID [12]byte
}

func (s *searchServer) serve(ctx context.Context, laddr *net.UDPAddr) (err error) {
	defer func() {
		if err != nil {
			ctxlog.L(ctx).Errorf("error listening for search requests: %v", err)
		}
	}()
	ln, err := udpconn.Listen("udp", laddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(ctx, laddr, conn)
	}
}

func (s *searchServer) handleConnection(ctx context.Context, laddr *net.UDPAddr, conn *udpconn.Conn) (err error) {
	defer func() {
		if err != nil {
			ctxlog.L(ctx).Warnf("error handling UDP packet: %v", err)
		}
	}()
	defer conn.Close()

	ctx = ctxlog.WithFields(ctx, ctxlog.Fields{
		"local_addr":  laddr,
		"remote_addr": conn.Addr,
	})

	c := connection.New(conn, proto.FLAG_FROM_SERVER)
	msg, err := c.Next(ctx)
	if err != nil {
		return err
	}

	ctxlog.L(ctx).Debugf("UDP packet received")

	if msg.Header.MessageCommand == proto.APP_SEARCH_REQUEST {
		var req proto.SearchRequest
		if err := msg.Decode(&req); err != nil {
			return err
		}
		// Process search
		// TODO: Send to local multicast group for other local apps

	}
	return io.EOF
}
