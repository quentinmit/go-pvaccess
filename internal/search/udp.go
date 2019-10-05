// search handles beacons, search requests and responses
package search

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net"
	"time"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/internal/udpconn"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

const startupInterval = time.Second
const startupCount = 15

// TODO: EPICS_PVA_BEACON_PERIOD environment variable
const beaconInterval = 5 * time.Second

// Server handles UDP beacons and searches.
type Server struct {
	// GUID is a randomly generated GUID that identifies the server.
	// It will be filled in by Serve if it is blank.
	GUID [12]byte
	// ServerAddr is the TCP address that the TCP server is listening on.
	ServerAddr *net.TCPAddr
}

// Serve transmits beacons and listens for searches on every interface on the machine.
func (s *Server) Serve(ctx context.Context) error {
	if _, err := rand.Read(s.GUID[:]); err != nil {
		return err
	}
	beacon := proto.BeaconMessage{
		GUID: s.GUID,
	}
	if len(s.ServerAddr.IP) > 0 {
		copy(beacon.ServerAddress[:], s.ServerAddr.IP.To16())
	}
	beacon.ServerPort = uint16(s.ServerAddr.Port)
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

	ln, err := udpconn.Listen(ctx)
	if err != nil {
		return err
	}

	beaconSender := connection.New(ln.BroadcastConn(), proto.FLAG_FROM_SERVER)
	beaconSender.Version = pvdata.PVByte(2)

	ctxlog.L(ctx).Infof("sending beacons to %v", ln.BroadcastSendAddresses())

	go func() {
		if err := s.serveSearch(ctx, ln); err != nil && err != io.EOF {
			ctxlog.L(ctx).Errorf("failed handling search request: %v", err)
		}
	}()

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

func (s *Server) serveSearch(ctx context.Context, ln *udpconn.Listener) (err error) {
	defer func() {
		if err != nil {
			ctxlog.L(ctx).Errorf("error listening for search requests: %v", err)
		}
	}()
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		laddr := conn.LocalAddr()
		ctx = ctxlog.WithFields(ctx, ctxlog.Fields{
			"local_addr": laddr,
			"proto":      "udp",
		})
		go s.handleConnection(ctx, ln, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, ln *udpconn.Listener, conn *udpconn.Conn) (err error) {
	defer func() {
		if err != nil && err != io.EOF {
			ctxlog.L(ctx).Warnf("error handling UDP packet: %v", err)
		}
	}()
	defer conn.Close()

	ctx = ctxlog.WithField(ctx, "remote_addr", conn.Addr())

	c := connection.New(conn, proto.FLAG_FROM_SERVER)
	c.Version = pvdata.PVByte(2)
	for {
		msg, err := c.Next(ctx)
		if err != nil {
			return err
		}
		switch msg.Header.MessageCommand {
		case proto.APP_ORIGIN_TAG:
			var req proto.OriginTag
			if err := msg.Decode(&req); err != nil {
				return err
			}
			forwarderAddress := net.IP(req.ForwarderAddress[:])
			if !forwarderAddress.IsUnspecified() {
				if !ln.IsTappedIP(forwarderAddress) {
					ctxlog.L(ctx).Infof("ignoring packet with an ORIGIN_TAG of %v that doesn't match a local address", forwarderAddress)
					return nil
				}
			}
		case proto.APP_SEARCH_REQUEST:
			var req proto.SearchRequest
			if err := msg.Decode(&req); err != nil {
				return err
			}
			ctxlog.L(ctx).Debugf("search request received: %#v", req)
			// Process search
			if req.Flags&proto.SEARCH_UNICAST == proto.SEARCH_UNICAST {
				var buf bytes.Buffer
				var localAddrArray [16]byte
				copy(localAddrArray[:], []byte(ln.LocalAddr().IP.To16()))
				fwdConn := connection.New(&buf, msg.Header.Flags&proto.FLAG_FROM_SERVER)
				fwdConn.SendApp(ctx, proto.APP_ORIGIN_TAG, &proto.OriginTag{
					ForwarderAddress: localAddrArray,
				})
				fwdReq := req
				fwdReq.Flags &= ^pvdata.PVUByte(proto.SEARCH_UNICAST)
				if net.IP(fwdReq.ResponseAddress[:]).IsUnspecified() {
					copy(fwdReq.ResponseAddress[:], conn.Addr().IP)
				}
				fwdConn.SendApp(ctx, proto.APP_SEARCH_REQUEST, &fwdReq)
				if _, err := ln.WriteMulticast(buf.Bytes()); err != nil {
					ctxlog.L(ctx).Warnf("failed to forward search to multicast group: %v", err)
				}
			}
			responseAddr := net.IP(req.ResponseAddress[:])
			if len(responseAddr) > 0 && !responseAddr.IsUnspecified() {
				conn.SetSendAddress(&net.UDPAddr{
					IP:   responseAddr,
					Port: int(req.ResponsePort),
				})
			}
			return s.Search(ctx, c, req)
		}
	}
}
