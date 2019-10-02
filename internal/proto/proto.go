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
	return pvdata.Encode(s, &v.Version, &v.Flags, &v.MessageCommand, &v.PayloadSize)
}
func (v *PVAccessHeader) PVDecode(s *pvdata.DecoderState) error {
	magic, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	if magic != MAGIC {
		return fmt.Errorf("unexpected magic %x", magic)
	}
	if err := pvdata.Decode(s, &v.Version, &v.Flags, &v.MessageCommand); err != nil {
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
	APP_CONNECTION_VALIDATED  = 0x09
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

type CreateChannelRequest_Channel struct {
	ClientChannelID pvdata.PVInt
	ChannelName     string `pvaccess:",bound=500"`
}
type CreateChannelRequest struct {
	Channels []CreateChannelRequest_Channel
}

func (c CreateChannelRequest) PVEncode(s *pvdata.EncoderState) error {
	// Encoded as a PVShort length (instead of PVSize)
	count := pvdata.PVShort(len(c.Channels))
	if err := pvdata.Encode(s, &count); err != nil {
		return err
	}
	for _, c := range c.Channels {
		if err := pvdata.Encode(s, &c); err != nil {
			return err
		}
	}
	return nil
}
func (c *CreateChannelRequest) PVDecode(s *pvdata.DecoderState) error {
	var count pvdata.PVShort
	if err := pvdata.Decode(s, &count); err != nil {
		return err
	}
	c.Channels = make([]CreateChannelRequest_Channel, int(count))
	for i := range c.Channels {
		if err := pvdata.Decode(s, &c.Channels[i]); err != nil {
			return err
		}
	}
	return nil
}

type CreateChannelResponse struct {
	ClientChannelID pvdata.PVInt
	ServerChannelID pvdata.PVInt
	Status          pvdata.PVStatus `pvaccess:",breakonerror"`
	AccessRights    pvdata.PVShort
}

// destroyChannelRequest
// destroyChannelResponse
// channelGetRequestInit
// channelGetResponseInit
// channelGetRequest
// channelGetResponse
// channelPutRequestInit
// channelPutResponseInit
// channelPutGetRequestInit
// channelPutGetResponseInit
// channelArrayRequestInit
// channelArrayResponseInit
// channelGetArrayRequest
// channelGetArrayResponse
// channelPutArrayRequest
// channelPutArrayResponse
// channelSetLengthRequest
// channelSetLengthResponse
// destroyRequest
// channelProcessRequestInit
// channelProcessResponseInit
// channelProcessRequest
// channelProcessResponse
// channelGetFieldRequest
// channelGetFieldResponse
// message

// Channel RPC

// Subcommands for ChannelRPCRequest
const (
	CHANNEL_RPC_INIT = 0x08
	// Destroy is a flag on top of another subcommand
	CHANNEL_RPC_DESTROY = 0x10
)

type ChannelRPCRequest struct {
	ServerChannelID pvdata.PVInt
	RequestID       pvdata.PVInt
	Subcommand      pvdata.PVByte
	PVRequest       pvdata.PVAny
}
type ChannelRPCResponseInit struct {
	RequestID  pvdata.PVInt
	Subcommand pvdata.PVByte `pvaccess:",always=0x08"`
	Status     pvdata.PVStatus
}
type ChannelRPCResponse struct {
	RequestID      pvdata.PVInt
	Subcommand     pvdata.PVByte
	Status         pvdata.PVStatus `pvaccess:",breakonerror"`
	PVResponseData pvdata.PVAny
}

// cancelRequest
// originTag
