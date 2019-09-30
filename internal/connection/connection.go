package connection

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"syscall"

	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

type Connection struct {
	Log       *log.Logger
	Version   pvdata.PVByte
	Direction pvdata.PVUByte

	conn           io.ReadWriter
	encoderState   *pvdata.EncoderState
	decoderState   *pvdata.DecoderState
	forceByteOrder bool
}

func New(conn io.ReadWriter, direction pvdata.PVUByte) *Connection {
	var prefix string
	if conn, ok := conn.(net.Conn); ok {
		prefix = fmt.Sprintf("[%s] ", conn.RemoteAddr())
	}
	return &Connection{
		Log:       log.New(os.Stderr, prefix, log.LstdFlags|log.Lshortfile),
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

func (c *Connection) flush() error {
	if f, ok := c.encoderState.Buf.(flusher); ok {
		if err := f.Flush(); err != nil {
			return err
		}
	}
	// TODO: Figure out if we ever need to add alignment bytes here (using AligningWriter).
	return nil
}

func (c *Connection) SendCtrl(messageCommand pvdata.PVByte, payloadSize pvdata.PVInt) error {
	defer c.flush()
	c.Log.Printf("sending control message %x with payload %x", messageCommand, payloadSize)
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

func (c *Connection) encodePayload(payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	defer c.encoderState.PushWriter(&buf)()
	if err := pvdata.Encode(c.encoderState, payload); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Connection) SendApp(messageCommand pvdata.PVByte, payload interface{}) error {
	defer c.flush()
	bytes, err := c.encodePayload(payload)
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
	c.Log.Printf("sending app message %x with payload size %d", messageCommand, len(bytes))
	if err := h.PVEncode(c.encoderState); err != nil {
		return err
	}
	_, err = c.encoderState.Buf.Write(bytes)
	return err
}

func (c *Connection) handleControlMessage(header *proto.PVAccessHeader) error {
	switch header.MessageCommand {
	case proto.CTRL_MARK_TOTAL_BYTE_SENT:
		return c.SendCtrl(proto.CTRL_ACK_TOTAL_BYTE_SENT, header.PayloadSize)
	case proto.CTRL_ACK_TOTAL_BYTE_SENT:
		// TODO: Implement flow control
	case proto.CTRL_SET_BYTE_ORDER:
		if header.PayloadSize == 0 {
			c.forceByteOrder = true
		}
	case proto.CTRL_ECHO_REQUEST:
		return c.SendCtrl(proto.CTRL_ECHO_RESPONSE, header.PayloadSize)
	default:
		c.Log.Printf("ignoring unknown control message %02x", header.MessageCommand)
	}
	return nil
}

type Message struct {
	Header proto.PVAccessHeader
	Data   []byte

	c      *Connection
	reader pvdata.Reader
}

func (c *Connection) Next() (*Message, error) {
	for {
		header := proto.PVAccessHeader{
			ForceByteOrder: c.forceByteOrder,
		}
		if err := pvdata.Decode(c.decoderState, &header); err != nil {
			return nil, err
		}
		c.Log.Printf("received packet %#v", header)
		if header.Flags&proto.FLAG_MSG_CTRL == proto.FLAG_MSG_CTRL {
			if err := c.handleControlMessage(&header); err != nil {
				return nil, err
			}
			continue
		}

		data := make([]byte, header.PayloadSize)
		if _, err := io.ReadFull(c.decoderState.Buf, data); err != nil {
			return &Message{Header: header, Data: data, c: c}, err
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
