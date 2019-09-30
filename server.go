package pvaccess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"syscall"
	"time"

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

type connection struct {
	log            *log.Logger
	version        pvdata.PVByte
	srv            *Server
	direction      pvdata.PVUByte
	conn           net.Conn
	writer         *proto.AligningWriter
	encoderState   *pvdata.EncoderState
	decoderState   *pvdata.DecoderState
	forceByteOrder bool
	channels       map[pvdata.PVInt]*connChannel
}

type connChannel struct {
	name string
}

func newConn(conn net.Conn) *connection {
	writer := proto.NewAligningWriter(conn)
	return &connection{
		log:    log.New(os.Stderr, fmt.Sprintf("[%s] ", conn.RemoteAddr()), log.LstdFlags|log.Lshortfile),
		conn:   conn,
		writer: writer,
		encoderState: &pvdata.EncoderState{
			Buf:       bufio.NewWriter(conn),
			ByteOrder: binary.LittleEndian,
		},
		decoderState: &pvdata.DecoderState{
			Buf: bufio.NewReader(conn),
		},
		channels: make(map[pvdata.PVInt]*connChannel),
	}
}

func (srv *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	c := newConn(conn)
	c.srv = srv
	if err := c.handleServer(); err != nil {
		c.log.Printf("error on connection %v: %v", conn.RemoteAddr(), err)
	}
}

type flusher interface {
	Flush() error
}

func (c *connection) AlignFlush() error {
	if f, ok := c.encoderState.Buf.(flusher); ok {
		if err := f.Flush(); err != nil {
			return err
		}
	}
	return nil
	n, err := c.writer.Align()
	if n != 0 {
		c.log.Printf("adding %d padding bytes", n)
	}
	return err
}

func (c *connection) sendCtrl(messageCommand pvdata.PVByte, payloadSize pvdata.PVInt) error {
	defer c.AlignFlush()
	c.log.Printf("sending control message %x with payload %x", messageCommand, payloadSize)
	flags := proto.FLAG_MSG_CTRL | c.direction
	if c.encoderState.ByteOrder == binary.BigEndian {
		flags |= proto.FLAG_BO_BE
	}
	h := proto.PVAccessHeader{
		Version:        c.version,
		Flags:          flags,
		MessageCommand: messageCommand,
		PayloadSize:    payloadSize,
	}
	return h.PVEncode(c.encoderState)
}

func (c *connection) encodePayload(payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	defer c.encoderState.PushWriter(&buf)()
	if err := pvdata.Encode(c.encoderState, payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *connection) sendApp(messageCommand pvdata.PVByte, payload interface{}) error {
	defer c.AlignFlush()
	bytes, err := c.encodePayload(payload)
	flags := proto.FLAG_MSG_APP | c.direction
	if c.encoderState.ByteOrder == binary.BigEndian {
		flags |= proto.FLAG_BO_BE
	}
	h := proto.PVAccessHeader{
		Version:        c.version,
		Flags:          flags,
		MessageCommand: messageCommand,
		PayloadSize:    pvdata.PVInt(len(bytes)),
	}
	c.log.Printf("sending app message %x with payload size %d", messageCommand, len(bytes))
	if err := h.PVEncode(c.encoderState); err != nil {
		return err
	}
	_, err = c.encoderState.Buf.Write(bytes)
	return err
}

func (c *connection) handleControlMessage(header *proto.PVAccessHeader) error {
	switch header.MessageCommand {
	case proto.CTRL_MARK_TOTAL_BYTE_SENT:
		return c.sendCtrl(proto.CTRL_ACK_TOTAL_BYTE_SENT, header.PayloadSize)
	case proto.CTRL_ACK_TOTAL_BYTE_SENT:
		// TODO: Implement flow control
	case proto.CTRL_SET_BYTE_ORDER:
		if header.PayloadSize == 0 {
			c.forceByteOrder = true
		}
	case proto.CTRL_ECHO_REQUEST:
		return c.sendCtrl(proto.CTRL_ECHO_RESPONSE, header.PayloadSize)
	default:
		c.log.Printf("ignoring unknown control message %02x", header.MessageCommand)
	}
	return nil
}

type filer interface {
	File() (*os.File, error)
}

func (c *connection) handleServer() error {
	c.direction = proto.FLAG_FROM_SERVER
	c.version = pvdata.PVByte(2)
	// 0 = Ignore byte order field in header
	if err := c.sendCtrl(proto.CTRL_SET_BYTE_ORDER, 0); err != nil {
		return err
	}
	bufSize := 32768 // default size if we can't fetch it
	if cf, ok := c.conn.(filer); ok {
		if file, err := cf.File(); err == nil {
			bufSize, _ = syscall.GetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
			file.Close()
		}
	}

	req := proto.ConnectionValidationRequest{
		ServerReceiveBufferSize:            pvdata.PVInt(bufSize),
		ServerIntrospectionRegistryMaxSize: 0x7fff,
		AuthNZ: []string{"anonymous"},
	}
	c.sendApp(proto.APP_CONNECTION_VALIDATION, &req)

	for {
		if err := c.handleServerOnePacket(); err != nil {
			return err
		}
	}
}
func (c *connection) handleServerOnePacket() error {
	header := proto.PVAccessHeader{
		ForceByteOrder: c.forceByteOrder,
	}
	if err := pvdata.Decode(c.decoderState, &header); err != nil {
		return err
	}
	c.log.Printf("received packet %#v", header)
	if header.Flags&proto.FLAG_MSG_CTRL == proto.FLAG_MSG_CTRL {
		return c.handleControlMessage(&header)
	}
	data := make([]byte, header.PayloadSize)
	if _, err := io.ReadFull(c.decoderState.Buf, data); err != nil {
		return err
	}
	// TODO: Segmented packets
	defer c.decoderState.PushReader(bytes.NewReader(data))()

	switch header.MessageCommand {
	case proto.APP_CONNECTION_VALIDATION:
		resp := proto.ConnectionValidationResponse{}
		if err := pvdata.Decode(c.decoderState, &resp); err != nil {
			return err
		}
		c.log.Printf("received connection validation %#v", resp)
		// TODO: Implement flow control
		if err := c.sendApp(proto.APP_CONNECTION_VALIDATED, &proto.ConnectionValidated{}); err != nil {
			return err
		}
	}
	if f, ok := serverDispatch[header.MessageCommand]; ok {
		return f(c)
	}
	return nil
}

var serverDispatch = map[pvdata.PVByte]func(c *connection) error{
	proto.APP_CHANNEL_CREATE: (*connection).handleCreateChannelRequest,
	proto.APP_CHANNEL_RPC:    (*connection).handleChannelRPC,
}

func (c *connection) handleCreateChannelRequest() error {
	var req proto.CreateChannelRequest
	if err := pvdata.Decode(c.decoderState, &req); err != nil {
		return err
	}
	var resp proto.CreateChannelResponse
	if len(req.Channels) == 1 {
		ch := req.Channels[0]
		c.log.Printf("received request to create channel %q as client ID %x", ch.ChannelName, ch.ClientChannelID)
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
	return c.sendApp(proto.APP_CHANNEL_CREATE, &resp)
}

func (c *connection) handleChannelRPC() error {
	var req proto.ChannelRPCRequest
	if err := pvdata.Decode(c.decoderState, &req); err != nil {
		return err
	}
	switch req.Subcommand {
	case proto.CHANNEL_RPC_INIT:
		c.log.Printf("received request to init channel RPC with body %#v", req.PVRequest)
	}
	return c.sendApp(proto.APP_CHANNEL_RPC, &proto.ChannelRPCResponseInit{
		RequestID:  req.RequestID,
		Subcommand: req.Subcommand,
		Status: pvdata.PVStatus{
			Type:    pvdata.PVStatus_ERROR,
			Message: pvdata.PVString("don't know how to execute that RPC"),
		},
	})
}
