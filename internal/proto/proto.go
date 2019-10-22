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

// echoRequest and echoResponse are raw bytes; version 1 servers always have an empty echoResponse.

// Search

const (
	SEARCH_REPLY_REQUIRED = 0x80
	SEARCH_UNICAST        = 0x01
)

type SearchRequest struct {
	SearchSequenceID pvdata.PVUInt
	Flags            pvdata.PVUByte // 0-bit for replyRequired, 7-th bit for "sent as unicast" (1)/"sent as broadcast/multicast" (0)

	Reserved [3]byte

	// if not provided (or zero), the same transport is used for responses
	// needs to be set when local broadcast (multicast on loop interface) is done
	ResponseAddress [16]byte        // e.g. IPv6 address in case of IP based transport, UDP
	ResponsePort    pvdata.PVUShort // e.g. socket port in case of IP based transport

	Protocols []pvdata.PVString

	Channels []SearchRequest_Channel `pvaccess:",short"`
}
type SearchRequest_Channel struct {
	SearchInstanceID pvdata.PVUInt
	ChannelName      string `pvaccess:",bound=500"`
}

type SearchResponse struct {
	GUID              [12]byte
	SearchSequenceID  pvdata.PVUInt
	ServerAddress     [16]byte        // e.g. IPv6 address in case of IP based transport
	ServerPort        pvdata.PVUShort // e.g. socket port in case of IP based transport
	Protocol          pvdata.PVString
	Found             pvdata.PVBoolean
	SearchInstanceIDs []pvdata.PVUInt `pvaccess:",short"`
}

// Create channel

type CreateChannelRequest_Channel struct {
	ClientChannelID pvdata.PVInt
	ChannelName     string `pvaccess:",bound=500"`
}
type CreateChannelRequest struct {
	Channels []CreateChannelRequest_Channel `pvaccess:",short"`
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

// Destroy Channel
type DestroyChannel struct {
	ServerChannelID, ClientChannelID pvdata.PVInt
}

// Channel *

// ChannelResponseError is the common struct used by all channel operations to report an error.
type ChannelResponseError struct {
	RequestID  pvdata.PVInt
	Subcommand pvdata.PVByte
	Status     pvdata.PVStatus
}

// Channel Get

const (
	CHANNEL_GET_INIT    = 0x08
	CHANNEL_GET_DESTROY = 0x10
	// 0x40 means "get" but is optional
)

type ChannelGetRequest struct {
	ServerChannelID pvdata.PVInt
	RequestID       pvdata.PVInt
	Subcommand      pvdata.PVByte
	// PVRequest is the requested fields, only present if Subcommand is CHANNEL_GET_INIT.
	PVRequest pvdata.PVAny
}

func (r ChannelGetRequest) PVEncode(s *pvdata.EncoderState) error {
	if err := pvdata.Encode(s, &r.ServerChannelID, &r.RequestID, &r.Subcommand); err != nil {
		return err
	}
	if r.Subcommand&CHANNEL_GET_INIT == CHANNEL_GET_INIT {
		return pvdata.Encode(s, &r.PVRequest)
	}
	return nil
}
func (r *ChannelGetRequest) PVDecode(s *pvdata.DecoderState) error {
	if err := pvdata.Decode(s, &r.ServerChannelID, &r.RequestID, &r.Subcommand); err != nil {
		return err
	}
	if r.Subcommand&CHANNEL_GET_INIT == CHANNEL_GET_INIT {
		return pvdata.Decode(s, &r.PVRequest)
	}
	return nil
}

type ChannelGetResponseInit struct {
	RequestID     pvdata.PVInt
	Subcommand    pvdata.PVByte
	Status        pvdata.PVStatus `pvaccess:",breakonerror"`
	PVStructureIF pvdata.FieldDesc
}

type ChannelGetResponse struct {
	RequestID  pvdata.PVInt
	Subcommand pvdata.PVByte
	Status     pvdata.PVStatus `pvaccess:",breakonerror"`
	// Value is the partial structure in the response.
	// On decode, Value.Value needs to be prepopulated with the struct to decode into.
	Value pvdata.PVStructureDiff
}

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

// Channel Monitor

// Channel Monitor subcommand flags
const (
	CHANNEL_MONITOR_INIT             = 0x08
	CHANNEL_MONITOR_PIPELINE_SUPPORT = 0x80
	CHANNEL_MONITOR_SUBSCRIPTION     = 0x04
	CHANNEL_MONITOR_SUBSCRIPTION_RUN = 0x40
	CHANNEL_MONITOR_TERMINATE        = 0x10
)

type ChannelMonitorRequest struct {
	ServerChannelID pvdata.PVInt
	RequestID       pvdata.PVInt
	Subcommand      pvdata.PVUByte
	// PVRequest is only present if CHANNEL_MONITOR_INIT
	PVRequest pvdata.PVAny
	// NFree is only present if CHANNEL_MONITOR_PIPELINE_SUPPORT
	NFree pvdata.PVInt
	// QueueSize is only present if CHANNEL_MONITOR_PIPELINE_SUPPORT
	QueueSize pvdata.PVInt
}

func (r ChannelMonitorRequest) PVEncode(s *pvdata.EncoderState) error {
	if err := pvdata.Encode(s, &r.ServerChannelID, &r.RequestID, &r.Subcommand); err != nil {
		return err
	}
	if r.Subcommand&CHANNEL_MONITOR_INIT == CHANNEL_MONITOR_INIT {
		if err := pvdata.Encode(s, &r.PVRequest); err != nil {
			return err
		}
	}
	if r.Subcommand&CHANNEL_MONITOR_PIPELINE_SUPPORT == CHANNEL_MONITOR_PIPELINE_SUPPORT {
		if err := pvdata.Encode(s, &r.NFree, &r.QueueSize); err != nil {
			return err
		}
	}
	return nil
}
func (r *ChannelMonitorRequest) PVDecode(s *pvdata.DecoderState) error {
	if err := pvdata.Decode(s, &r.ServerChannelID, &r.RequestID, &r.Subcommand); err != nil {
		return err
	}
	if r.Subcommand&CHANNEL_MONITOR_INIT == CHANNEL_MONITOR_INIT {
		if err := pvdata.Decode(s, &r.PVRequest); err != nil {
			return err
		}
	}
	if r.Subcommand&CHANNEL_MONITOR_PIPELINE_SUPPORT == CHANNEL_MONITOR_PIPELINE_SUPPORT {
		if err := pvdata.Decode(s, &r.NFree, &r.QueueSize); err != nil {
			return err
		}
	}
	return nil
}

type ChannelMonitorResponseInit struct {
	RequestID     pvdata.PVInt
	Subcommand    pvdata.PVByte
	Status        pvdata.PVStatus `pvaccess:",breakonerror"`
	PVStructureIF pvdata.FieldDesc
}
type ChannelMonitorResponse struct {
	RequestID  pvdata.PVInt
	Subcommand pvdata.PVByte
	// Value is the partial structure in the response.
	// On decode, Value.Value needs to be prepopulated with the struct to decode into.
	Value         pvdata.PVStructureDiff
	OverrunBitSet pvdata.PVBitSet
}

// Cancel Request and Destroy Request
type CancelDestroyRequest struct {
	ServerChannelID pvdata.PVInt
	RequestID       pvdata.PVInt
}

// Origin Tag
type OriginTag struct {
	ForwarderAddress [16]byte
}
