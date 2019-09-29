package pvaccess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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
	version        pvdata.PVByte
	srv            *Server
	direction      pvdata.PVByte
	conn           net.Conn
	encoderState   *pvdata.EncoderState
	decoderState   *pvdata.DecoderState
	forceByteOrder bool
}

func (srv *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	c := &connection{
		srv:       srv,
		direction: proto.FLAG_FROM_SERVER,
		conn:      conn,
		encoderState: &pvdata.EncoderState{
			Buf:       bufio.NewWriter(conn),
			ByteOrder: binary.LittleEndian,
		},
		decoderState: &pvdata.DecoderState{
			Buf: bufio.NewReader(conn),
		},
	}
	if err := c.handleServer(); err != nil {
		log.Printf("error on connection %v: %v", conn, err)
	}
}

func (c *connection) Flush() error {
	return c.encoderState.Buf.(*bufio.Writer).Flush()
}

func (c *connection) sendCtrl(messageCommand pvdata.PVByte, payloadSize pvdata.PVInt) error {
	defer c.Flush()
	h := proto.PVAccessHeader{
		Version:        c.version,
		Flags:          proto.FLAG_MSG_CTRL | c.direction,
		MessageCommand: messageCommand,
		PayloadSize:    payloadSize,
	}
	return h.PVEncode(c.encoderState)
}

func (c *connection) encodePayload(payload interface{}) ([]byte, error) {
	oldBuf := c.encoderState.Buf
	defer func() {
		c.encoderState.Buf = oldBuf
	}()
	var buf bytes.Buffer
	c.encoderState.Buf = &buf
	if err := pvdata.Encode(c.encoderState, payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *connection) sendApp(messageCommand pvdata.PVByte, payload interface{}) error {
	defer c.Flush()
	bytes, err := c.encodePayload(payload)
	h := proto.PVAccessHeader{
		Version:        c.version,
		Flags:          proto.FLAG_MSG_APP | c.direction,
		MessageCommand: messageCommand,
		PayloadSize:    pvdata.PVInt(len(bytes)),
	}
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
		log.Printf("ignoring unknown control message %02x", header.MessageCommand)
	}
	return nil
}

type filer interface {
	File() (*os.File, error)
}

func (c *connection) handleServer() error {
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
		ServerIntrospectionRegistryMaxSize: 1024,
		AuthNZ: []string{"anonymous"},
	}
	c.sendApp(proto.APP_CONNECTION_VALIDATION, req)

	for {
		header := proto.PVAccessHeader{
			ForceByteOrder: c.forceByteOrder,
		}
		if err := pvdata.Decode(c.decoderState, &header); err != nil {
			return err
		}
		log.Printf("received packet %#v", header)
		if header.Flags&proto.FLAG_MSG_CTRL == proto.FLAG_MSG_CTRL {
			if err := c.handleControlMessage(&header); err != nil {
				return err
			}
			continue
		}
		switch header.MessageCommand {
		case proto.APP_CONNECTION_VALIDATION:
			resp := proto.ConnectionValidationResponse{}
			if err := pvdata.Decode(c.decoderState, &resp); err != nil {
				return err
			}
			// TODO: Implement flow control
		}
	}
}
