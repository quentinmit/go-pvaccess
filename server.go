package pvaccess

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
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
		go srv.handleConnection(ctx, conn)
	}
}

type serverConn struct {
	*connection.Connection
	srv *Server

	mu       sync.Mutex
	channels map[pvdata.PVInt]*connChannel
}

type connChannel struct {
	name      string
	handleRPC func(ctx context.Context, args pvdata.PVStructure) (response interface{}, status pvdata.PVStatus)
}

func (srv *Server) newConn(conn io.ReadWriter) *serverConn {
	c := connection.New(conn, proto.FLAG_FROM_SERVER)
	return &serverConn{
		Connection: c,
		srv:        srv,
		channels:   make(map[pvdata.PVInt]*connChannel),
	}
}

func (srv *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	c := srv.newConn(conn)
	c.Log.Printf("new connection")
	if err := c.serve(ctx); err != nil {
		c.Log.Printf("error on connection %v: %v", conn.RemoteAddr(), err)
	}
}

func (c *serverConn) serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
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
		if err := c.handleServerOnePacket(ctx); err != nil {
			if err == io.EOF {
				cancel()
				// TODO: Cleanup resources (requests, channels, etc.)
				c.Log.Printf("client went away, closing connection")
				return nil
			}
			return err
		}
	}
}
func (c *serverConn) handleServerOnePacket(ctx context.Context) error {
	msg, err := c.Next()
	if err != nil {
		return err
	}
	if f, ok := serverDispatch[msg.Header.MessageCommand]; ok {
		return f(c, ctx, msg)
	}
	return nil
}

var serverDispatch = map[pvdata.PVByte]func(c *serverConn, ctx context.Context, msg *connection.Message) error{
	proto.APP_CONNECTION_VALIDATION: (*serverConn).handleConnectionValidation,
	proto.APP_CHANNEL_CREATE:        (*serverConn).handleCreateChannelRequest,
	proto.APP_CHANNEL_RPC:           (*serverConn).handleChannelRPC,
}

func (c *serverConn) handleConnectionValidation(_ context.Context, msg *connection.Message) error {
	var resp proto.ConnectionValidationResponse
	if err := msg.Decode(&resp); err != nil {
		return err
	}
	c.Log.Printf("received connection validation %#v", resp)
	// TODO: Implement flow control
	return c.SendApp(proto.APP_CONNECTION_VALIDATED, &proto.ConnectionValidated{})
}

func (c *serverConn) handleCreateChannelRequest(_ context.Context, msg *connection.Message) error {
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
			c.mu.Lock()
			c.channels[ch.ClientChannelID] = &connChannel{
				name:      ch.ChannelName,
				handleRPC: c.handleServerRPC,
			}
			c.mu.Unlock()
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

func (c *serverConn) handleServerRPC(_ context.Context, args pvdata.PVStructure) (response interface{}, status pvdata.PVStatus) {
	if strings.HasPrefix(args.ID, "epics:nt/NTURI:1.") {
		if q, ok := args.SubField("query").(*pvdata.PVStructure); ok {
			args = *q
		} else {
			return struct{}{}, pvdata.PVStatus{
				Type:    pvdata.PVStatus_ERROR,
				Message: pvdata.PVString("invalid argument"),
			}
		}
	}

	if args.SubField("help") != nil {
		// TODO
	}

	var op pvdata.PVString
	if v, ok := args.SubField("op").(*pvdata.PVString); ok {
		op = *v
	}

	c.Log.Printf("op = %s", op)

	switch op {
	case "channels":
	case "info":
		hostname, _ := os.Hostname()
		info := &struct {
			Process   string `pvaccess:"process"`
			StartTime string `pvaccess:"startTime"`
			Version   string `pvaccess:"version"`
			ImplLang  string `pvaccess:"implLang"`
			Host      string `pvaccess:"host"`
			OS        string `pvaccess:"os"`
			Arch      string `pvaccess:"arch"`
		}{
			os.Args[0],
			"sometime",
			"1.0",
			"Go",
			hostname,
			runtime.GOOS,
			runtime.GOARCH,
		}
		c.Log.Printf("returning info %+v", info)
		return info, pvdata.PVStatus{}
	}

	return &struct{}{}, pvdata.PVStatus{
		Type:    pvdata.PVStatus_ERROR,
		Message: pvdata.PVString("invalid argument"),
	}
}

func (c *serverConn) handleChannelRPC(ctx context.Context, msg *connection.Message) error {
	var req proto.ChannelRPCRequest
	if err := msg.Decode(&req); err != nil {
		return err
	}
	c.Log.Printf("CHANNEL_RPC(%#v)", req)
	resp := &proto.ChannelRPCResponseInit{
		RequestID:  req.RequestID,
		Subcommand: req.Subcommand,
	}
	c.mu.Lock()
	channel := c.channels[req.ServerChannelID]
	c.mu.Unlock()
	c.Log.Printf("channel = %#v", channel)
	if channel != nil && channel.handleRPC != nil {
		switch req.Subcommand {
		case proto.CHANNEL_RPC_INIT:
			c.Log.Printf("received request to init channel RPC with body %v", req.PVRequest.Data)
			return c.SendApp(proto.APP_CHANNEL_RPC, resp)
		}
		c.Log.Printf("%T", req.PVRequest.Data)
		if args, ok := req.PVRequest.Data.(pvdata.PVStructure); ok {
			c.Log.Printf("received request to execute channel RPC with body %v", args)
			resp, status := channel.handleRPC(ctx, args)
			r := &proto.ChannelRPCResponse{
				RequestID:      req.RequestID,
				Subcommand:     req.Subcommand,
				Status:         status,
				PVResponseData: pvdata.NewPVAny(resp),
			}
			return c.SendApp(proto.APP_CHANNEL_RPC, r)
		}
	}
	c.Log.Printf("request to RPC on channel %q which cannot RPC", channel.name)
	resp.Status = pvdata.PVStatus{
		Type:    pvdata.PVStatus_ERROR,
		Message: pvdata.PVString("don't know how to execute that RPC"),
	}

	return c.SendApp(proto.APP_CHANNEL_RPC, resp)
}
