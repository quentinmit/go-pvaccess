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

// Serve transmits beacons and listens for searches on every interface on the machine.
// If serverAddr specifies an IP, beacons will advertise that address.
// If it does not, beacons will advertise the address of the interface they are transmitted on.
func Serve(ctx context.Context, serverAddr *net.TCPAddr) error {
	var beacon proto.BeaconMessage
	if _, err := rand.Read(beacon.GUID[:]); err != nil {
		return err
	}
	if len(serverAddr.IP) > 0 {
		// TODO: How should IPv4 addresses be aligned?
		copy(beacon.ServerAddress[:], serverAddr.IP)
	}
	beacon.ServerPort = uint16(serverAddr.Port)
	beacon.Protocol = "tcp"

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

	sendConn, err := udpconn.NewMulti()
	if err != nil {
		return err
	}
	beaconSender := connection.New(sendConn, proto.FLAG_FROM_SERVER)

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
				}
				ips = append(ips, laddr)

				var extra []*net.UDPAddr

				if i.Flags&net.FlagBroadcast == net.FlagBroadcast {
					laddr := &net.UDPAddr{
						IP:   bcastIP(addr.IP, addr.Mask),
						Port: laddr.Port,
						Zone: laddr.Zone,
					}
					ips = append(ips, laddr)
					bcasts = append(bcasts, laddr)
					extra = append(extra, laddr)
				}
				ctxlog.L(ctx).Debugf("Listening on %v + %v", laddr, extra)

				go func() {
					if err := (&searchServer{
						GUID: beacon.GUID,
					}).serve(ctx, laddr, extra); err != nil && err != io.EOF {
						ctxlog.L(ctx).Errorf("failed handling search request: %v", err)
					}
				}()
			}
		}
	}
	sendConn.SendAddresses = bcasts
	ctxlog.L(ctx).Infof("sending beacons to %v", bcasts)
	ctxlog.L(ctx).Infof("listening on %v", ips)
	/*
		go (&searchServer{
			GUID: beacon.GUID,
		}).serve(ctx, &net.UDPAddr{Port: udpPort})
	*/
	ticker := time.NewTicker(startupInterval)
	defer func() { ticker.Stop() }()
	i := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			beacon.BeaconSequenceID++
			beaconSender.SendApp(ctx, proto.APP_BEACON, &beacon)
			i++
			if i == startupCount {
				ticker.Stop()
				ticker = time.NewTicker(beaconInterval)
			}
		}
	}
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

type searchServer struct {
	GUID [12]byte
}

func (s *searchServer) serve(ctx context.Context, laddr *net.UDPAddr, extra []*net.UDPAddr) (err error) {
	defer func() {
		if err != nil {
			ctxlog.L(ctx).Errorf("error listening for search requests: %v", err)
		}
	}()
	ctx = ctxlog.WithField(ctx, "local_addr", laddr)
	ln, err := udpconn.Listen(ctx, "udp", laddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	for _, e := range extra {
		if err := ln.AddReadAddress(ctx, "udp", e); err != nil {
			return err
		}
	}
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
		if err != nil && err != io.EOF {
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
