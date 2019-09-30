package pvaccess

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/quentinmit/go-pvaccess/internal/connection"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

type Server struct {
}

const udpAddr = ":5076"

// TODO: Pick a random TCP port for each server and announce it in beacons
const tcpAddr = ":5075"

func (srv *Server) ListenAndServe(ctx context.Context) error {
	addr := tcpAddr
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(ctx, ln)
}

// TODO: UDP beacon support
func (srv *Server) Serve(ctx context.Context, l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return err
		}
		go srv.handleConnection(conn)
	}
}

type serverConn struct {
	*connection.Connection
	srv      *Server
	channels map[pvdata.PVInt]*connChannel
}

type connChannel struct {
	name string
}

func (srv *Server) newConn(conn io.ReadWriter) *serverConn {
	c := connection.New(conn, proto.FLAG_FROM_SERVER)
	return &serverConn{
		Connection: c,
		srv:        srv,
		channels:   make(map[pvdata.PVInt]*connChannel),
	}
}

func (srv *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	c := srv.newConn(conn)
	if err := c.serve(); err != nil {
		c.Log.Printf("error on connection %v: %v", conn.RemoteAddr(), err)
	}
}

func (c *serverConn) serve() error {
	c.Version = pvdata.PVByte(2)
	// 0 = Ignore byte order field in header
	if err := c.SendCtrl(proto.CTRL_SET_BYTE_ORDER, 0); err != nil {
		return err
	}

	req := proto.ConnectionValidationRequest{
		ServerReceiveBufferSize:            pvdata.PVInt(c.ReceiveBufferSize()),
		ServerIntrospectionRegistryMaxSize: 0x7fff,
		AuthNZ: []string{"anonymous"},
	}
	c.SendApp(proto.APP_CONNECTION_VALIDATION, &req)

	for {
		if err := c.handleServerOnePacket(); err != nil {
			return err
		}
	}
}
func (c *serverConn) handleServerOnePacket() error {
	msg, err := c.Next()
	if err != nil {
		return err
	}
	if f, ok := serverDispatch[msg.Header.MessageCommand]; ok {
		return f(c, msg)
	}
	return nil
}

var serverDispatch = map[pvdata.PVByte]func(c *serverConn, msg *connection.Message) error{
	proto.APP_CONNECTION_VALIDATION: (*serverConn).handleConnectionValidation,
	proto.APP_CHANNEL_CREATE:        (*serverConn).handleCreateChannelRequest,
	proto.APP_CHANNEL_RPC:           (*serverConn).handleChannelRPC,
}

func (c *serverConn) handleConnectionValidation(msg *connection.Message) error {
	var resp proto.ConnectionValidationResponse
	if err := msg.Decode(&resp); err != nil {
		return err
	}
	c.Log.Printf("received connection validation %#v", resp)
	// TODO: Implement flow control
	return c.SendApp(proto.APP_CONNECTION_VALIDATED, &proto.ConnectionValidated{})
}

func (c *serverConn) handleCreateChannelRequest(msg *connection.Message) error {
	var req proto.CreateChannelRequest
	if err := msg.Decode(&req); err != nil {
		return err
	}
	var resp proto.CreateChannelResponse
	if len(req.Channels) == 1 {
		ch := req.Channels[0]
		c.Log.Printf("received request to create channel %q as client ID %x", ch.ChannelName, ch.ClientChannelID)
		resp.ClientChannelID = ch.ClientChannelID
		if ch.ChannelName == "server" {
			resp.ServerChannelID = ch.ClientChannelID
			c.channels[ch.ClientChannelID] = &connChannel{
				name: ch.ChannelName,
			}
		} else {
			resp.Status.Type = pvdata.PVStatus_ERROR
			resp.Status.Message = pvdata.PVString(fmt.Sprintf("unknown channel %q", ch.ChannelName))
		}
	} else {
		resp.Status.Type = pvdata.PVStatus_ERROR
		resp.Status.Message = "wrong number of channels"
	}
	return c.SendApp(proto.APP_CHANNEL_CREATE, &resp)
}

func (c *serverConn) handleChannelRPC(msg *connection.Message) error {
	var req proto.ChannelRPCRequest
	if err := msg.Decode(&req); err != nil {
		return err
	}
	switch req.Subcommand {
	case proto.CHANNEL_RPC_INIT:
		c.Log.Printf("received request to init channel RPC with body %#v", req.PVRequest)
	}
	return c.SendApp(proto.APP_CHANNEL_RPC, &proto.ChannelRPCResponseInit{
		RequestID:  req.RequestID,
		Subcommand: req.Subcommand,
		Status: pvdata.PVStatus{
			Type:    pvdata.PVStatus_ERROR,
			Message: pvdata.PVString("don't know how to execute that RPC"),
		},
	})
}
