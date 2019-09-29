package proto

import (
	"encoding/binary"
	"fmt"

	"github.com/quentinmit/go-pvaccess/pvdata"
)

// TODO: Messages MUST be aligned on a 64-bit boundary (Pad with zeros???)

const MAGIC = 0xCA

const (
	FLAG_MSG_APP        = 0
	FLAG_MSG_CTRL       = 1
	FLAG_SEGMENT_FIRST  = 0x08
	FLAG_SEGMENT_LAST   = 0x10
	FLAG_SEGMENT_MIDDLE = 0x18
	FLAG_FROM_CLIENT    = 0x00
	FLAG_FROM_SERVER    = 0x40
	FLAG_BO_LE          = 0x00
	FLAG_BO_BE          = 0x80
)

type PVAccessHeader struct {
	Version        pvdata.PVByte
	Flags          pvdata.PVUByte
	MessageCommand pvdata.PVByte
	PayloadSize    pvdata.PVInt
	ForceByteOrder bool
}

func (v *PVAccessHeader) PVEncode(s *pvdata.EncoderState) error {
	if err := s.Buf.WriteByte(MAGIC); err != nil {
		return err
	}
	if err := v.Version.PVEncode(s); err != nil {
		return err
	}
	if err := v.Flags.PVEncode(s); err != nil {
		return err
	}
	if err := v.MessageCommand.PVEncode(s); err != nil {
		return err
	}
	return v.PayloadSize.PVEncode(s)
}
func (v *PVAccessHeader) PVDecode(s *pvdata.DecoderState) error {
	magic, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	if magic != MAGIC {
		return fmt.Errorf("unexpected magic %x", magic)
	}
	if err := v.Version.PVDecode(s); err != nil {
		return err
	}
	if err := v.Flags.PVDecode(s); err != nil {
		return err
	}
	if err := v.MessageCommand.PVDecode(s); err != nil {
		return err
	}
	// Need to decode flags before decoding PayloadSize
	if !v.ForceByteOrder {
		if v.Flags&0x70 == 0x70 {
			s.ByteOrder = binary.BigEndian
		} else {
			s.ByteOrder = binary.LittleEndian
		}
	}
	return v.PayloadSize.PVDecode(s)
}

const (
	APP_BEACON                = 0x00
	APP_CONNECTION_VALIDATION = 0x01
	APP_ECHO                  = 0x02
	APP_SEARCH_REQUEST        = 0x03
	APP_SEARCH_RESPONSE       = 0x04
	APP_CHANNEL_CREATE        = 0x07
	APP_CHANNEL_DESTROY       = 0x08
	APP_CHANNEL_GET           = 0x0A
	APP_CHANNEL_PUT           = 0x0B
	APP_CHANNEL_PUT_GET       = 0x0C
	APP_CHANNEL_MONITOR       = 0x0D
	APP_CHANNEL_ARRAY         = 0x0E
	APP_REQUEST_DESTROY       = 0x0F
	APP_CHANNEL_PROCESS       = 0x10
	APP_CHANNEL_INTROSPECTION = 0x11
	APP_MESSAGE               = 0x12
	APP_CHANNEL_RPC           = 0x14
	APP_REQUEST_CANCEL        = 0x15
	APP_ORIGIN_TAG            = 0x16
	// Control messages do not have a body, but instead use the payload size as data.
	CTRL_MARK_TOTAL_BYTE_SENT = 0x00
	CTRL_ACK_TOTAL_BYTE_SENT  = 0x01
	CTRL_SET_BYTE_ORDER       = 0x02
	CTRL_ECHO_REQUEST         = 0x03
	CTRL_ECHO_RESPONSE        = 0x04
)

type BeaconMessage struct {
	GUID             [12]byte
	Flags            byte
	BeaconSequenceID byte
	ChangeCount      int16
	ServerAddress    [16]byte
	ServerPort       uint16
	Protocol         string
	ServerStatus     pvdata.PVAny
}

type ConnectionValidationRequest struct {
	ServerReceiveBufferSize            pvdata.PVInt
	ServerIntrospectionRegistryMaxSize pvdata.PVShort
	// AuthNZ is the list of supported authNZ methods.
	AuthNZ []string
}

type ConnectionValidationResponse struct {
	ClientReceiveBufferSize            pvdata.PVInt
	ClientIntrospectionRegistryMaxSize pvdata.PVShort
	ConnectionQos                      pvdata.PVShort
	// AuthNZ is the selected authNZ method.
	AuthNZ pvdata.PVString
	// PVAny is optional, content depends on authNZ
	Data pvdata.PVAny
}

type ConnectionValidated struct {
	Status pvdata.PVStatus
}
