package connection

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/quentinmit/go-pvaccess/internal/ctxlog"
	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

type Connection struct {
	Version   pvdata.PVByte
	Direction pvdata.PVUByte

	conn io.ReadWriter
	// encoderMu protects use of encoderState.
	encoderMu      sync.Mutex
	encoderState   *pvdata.EncoderState
	decoderState   *pvdata.DecoderState
	forceByteOrder bool
}

func New(conn io.ReadWriter, direction pvdata.PVUByte) *Connection {
	return &Connection{
		Direction: direction,
		conn:      conn,
		encoderState: &pvdata.EncoderState{
			Buf:       bufio.NewWriter(conn),
			ByteOrder: binary.LittleEndian,
		},
		decoderState: &pvdata.DecoderState{
			Buf: bufio.NewReader(conn),
		},
	}
}

type filer interface {
	File() (*os.File, error)
}

func (c *Connection) ReceiveBufferSize() int {
	bufSize := 32768 // default size if we can't fetch it
	if cf, ok := c.conn.(filer); ok {
		if file, err := cf.File(); err == nil {
			bufSize, _ = syscall.GetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
			file.Close()
		}
	}
	return bufSize
}

type flusher interface {
	Flush() error
}

// flush must be called with encoderMu held.
func (c *Connection) flush() error {
	if f, ok := c.encoderState.Buf.(flusher); ok {
		if err := f.Flush(); err != nil {
			return err
		}
	}
	// TODO: Figure out if we ever need to add alignment bytes here (using AligningWriter).
	return nil
}

// SendCtrl sends a control message on the wire.
// It is safe to call SendCtrl from any goroutine.
func (c *Connection) SendCtrl(ctx context.Context, messageCommand pvdata.PVByte, payloadSize pvdata.PVInt) error {
	c.encoderMu.Lock()
	defer c.encoderMu.Unlock()
	defer c.flush()
	ctxlog.L(ctx).WithFields(ctxlog.Fields{
		"command":      messageCommand,
		"payload_size": payloadSize,
	}).Debug("sending control message")
	flags := proto.FLAG_MSG_CTRL | c.Direction
	if c.encoderState.ByteOrder == binary.BigEndian {
		flags |= proto.FLAG_BO_BE
	}
	h := proto.PVAccessHeader{
		Version:        c.Version,
		Flags:          flags,
		MessageCommand: messageCommand,
		PayloadSize:    payloadSize,
	}
	return h.PVEncode(c.encoderState)
}

// encodePayload must be called with encoderMu held.
func (c *Connection) encodePayload(payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	defer c.encoderState.PushWriter(&buf)()
	if err := pvdata.Encode(c.encoderState, payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SendApp sends an application message on the wire.
// payload must be something that can be passed to pvdata.Encode; i.e. it must be either an instance of PVField or a pointer to something that can be converted to a PVField.
// If payload is a []byte it will be sent raw.
// It is safe to call SendApp from any goroutine.
func (c *Connection) SendApp(ctx context.Context, messageCommand pvdata.PVByte, payload interface{}) error {
	c.encoderMu.Lock()
	defer c.encoderMu.Unlock()
	defer c.flush()
	var bytes []byte
	if b, ok := payload.([]byte); ok {
		bytes = b
	} else {
		var err error
		bytes, err = c.encodePayload(payload)
		if err != nil {
			return err
		}
	}
	flags := proto.FLAG_MSG_APP | c.Direction
	if c.encoderState.ByteOrder == binary.BigEndian {
		flags |= proto.FLAG_BO_BE
	}
	h := proto.PVAccessHeader{
		Version:        c.Version,
		Flags:          flags,
		MessageCommand: messageCommand,
		PayloadSize:    pvdata.PVInt(len(bytes)),
	}
	l := ctxlog.L(ctx).WithFields(ctxlog.Fields{
		"command":      messageCommand,
		"payload_size": len(bytes),
	})
	l.Debug("sending app message")
	l.Tracef("app message body = %x", bytes)
	if err := h.PVEncode(c.encoderState); err != nil {
		return err
	}
	_, err := c.encoderState.Buf.Write(bytes)
	return err
}

func (c *Connection) handleControlMessage(ctx context.Context, header *proto.PVAccessHeader) error {
	ctx = ctxlog.WithField(ctx, "request_command", header.MessageCommand)
	switch header.MessageCommand {
	case proto.CTRL_MARK_TOTAL_BYTE_SENT:
		return c.SendCtrl(ctx, proto.CTRL_ACK_TOTAL_BYTE_SENT, header.PayloadSize)
	case proto.CTRL_ACK_TOTAL_BYTE_SENT:
		// TODO: Implement flow control
	case proto.CTRL_SET_BYTE_ORDER:
		if header.PayloadSize == 0 {
			c.forceByteOrder = true
		}
	case proto.CTRL_ECHO_REQUEST:
		return c.SendCtrl(ctx, proto.CTRL_ECHO_RESPONSE, header.PayloadSize)
	default:
		ctxlog.L(ctx).Warnf("ignoring unknown control message %02x", header.MessageCommand)
	}
	return nil
}

func (c *Connection) handleAppEcho(ctx context.Context, header proto.PVAccessHeader, data []byte) error {
	if header.Version >= 2 {
		return c.SendApp(ctx, proto.APP_ECHO, data)
	}
	return c.SendApp(ctx, proto.APP_ECHO, []byte{})
}

type Message struct {
	Header proto.PVAccessHeader
	Data   []byte

	c      *Connection
	reader pvdata.Reader
}

func (c *Connection) Next(ctx context.Context) (*Message, error) {
	for {
		header := proto.PVAccessHeader{
			ForceByteOrder: c.forceByteOrder,
		}
		if err := pvdata.Decode(c.decoderState, &header); err != nil {
			return nil, err
		}
		ctxlog.L(ctx).WithFields(ctxlog.Fields{
			"version":         header.Version,
			"flags":           header.Flags,
			"message_command": header.MessageCommand,
			"payload_size":    header.PayloadSize,
		}).Debug("received packet")
		if header.Flags&proto.FLAG_MSG_CTRL == proto.FLAG_MSG_CTRL {
			if err := c.handleControlMessage(ctx, &header); err != nil {
				return nil, err
			}
			continue
		}

		data := make([]byte, header.PayloadSize)
		if _, err := io.ReadFull(c.decoderState.Buf, data); err != nil {
			return &Message{Header: header, Data: data, c: c}, err
		}

		if header.MessageCommand == proto.APP_ECHO {
			if err := c.handleAppEcho(ctx, header, data); err != nil {
				return nil, err
			}
			continue
		}
		// TODO: Segmented packets
		return &Message{Header: header, Data: data, c: c}, nil
	}
}

// Decode decodes data from msg into out using the connection's established decoder state.
func (msg *Message) Decode(out interface{}) error {
	if msg.reader == nil {
		msg.reader = bytes.NewReader(msg.Data)
	}
	defer msg.c.decoderState.PushReader(msg.reader)()
	return pvdata.Decode(msg.c.decoderState, out)
}
