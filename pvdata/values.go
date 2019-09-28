package pvdata

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"reflect"
)

const max8 = 254
const max32 = 0x7fffffff

func check(n int, err error) error {
	return err
}

type Writer interface {
	io.Writer
	io.ByteWriter
	io.StringWriter
}

type EncoderState struct {
	Buf       Writer
	ByteOrder binary.ByteOrder
}

func (s *EncoderState) WriteUint16(v uint32) error {
	var bytes [2]byte
	s.ByteOrder.PutUint16(bytes, v)
	return check(s.Buf.Write(bytes))
}
func (s *EncoderState) WriteUint32(v uint32) error {
	var bytes [4]byte
	s.ByteOrder.PutUint32(bytes, v)
	return check(s.Buf.Write(bytes))
}
func (s *EncoderState) WriteUint64(v uint64) error {
	var bytes [8]byte
	s.ByteOrder.PutUint64(bytes, v)
	return check(s.Buf.Write(bytes))
}

type DecoderState struct {
	Buf       *bufio.Reader
	ByteOrder binary.ByteOrder
}

func (s *DecoderState) ReadUint16() (uint16, error) {
	var bytes [2]byte
	if err := io.ReadFull(s.Buf, bytes[:]); err != nil {
		return 0, err
	}
	return s.ByteOrder.Uint16(bytes), nil
}
func (s *DecoderState) ReadUint32() (uint32, error) {
	var bytes [4]byte
	if err := io.ReadFull(s.Buf, bytes[:]); err != nil {
		return 0, err
	}
	return s.ByteOrder.Uint32(bytes), nil
}
func (s *DecoderState) ReadUint64() (uint64, error) {
	var bytes [8]byte
	if err := io.ReadFull(s.Buf, bytes[:]); err != nil {
		return 0, err
	}
	return s.ByteOrder.Uint64(bytes), nil
}

type PVField interface {
	PVEncode(s *EncoderState) error
	PVDecode(s *DecoderState) error
}

// Size (special rules)

type PVSize int64

func (v *PVSize) PVEncode(s *EncoderState) error {
	if *v < 0 {
		return s.Buf.WriteByte(255)
	}
	if *v < 8 {
		return s.Buf.WriteByte(byte(*v))
	}
	if *v < max32 {
		out := make([]byte, 5)
		out[0] = max8
		s.ByteOrder.PutUint32(out[1:], uint32(*v))
		return check(s.Buf.Write(out))
	}
	out := make([]byte, 13)
	out[0] = max8
	s.ByteOrder.PutUint32(out[1:5], max32)
	s.ByteOrder.PutUint64(out[6:], uint64(*v))
	return check(s.Buf.Write(out))
}
func (v *PVSize) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	if data == 255 {
		*v = -1
		return nil
	}
	if data != max8 {
		*v = PVSize(data)
		return nil
	}
	data32, err := s.ReadUint32()
	if err != nil {
		return err
	}
	if data32 != max32 {
		*v = PVSize(data32)
		return nil
	}
	data64, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVSize(data64)
	return nil
}

// Basic types (encode as normal, paying attention to endianness)

type PVBoolean bool

func (v *PVBoolean) PVEncode(s *EncoderState) error {
	if *v {
		return s.Buf.WriteByte(1)
	}
	return s.Buf.WriteByte(0)
}
func (v *PVBoolean) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = (data != 0)
}

type PVByte int8

func (v *PVByte) PVEncode(s *EncoderState) error {
	return s.Buf.WriteByte(byte(*v))
}
func (v *PVByte) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = PVByte(data)
}

type PVUByte uint8

func (v *PVUByte) PVEncode(s *EncoderState) error {
	return s.Buf.WriteByte(*v)
}
func (v *PVUByte) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = data
}

type PVShort int16

func (v *PVShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(*v))
}
func (v *PVShort) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVShort(data)
}

type PVUShort uint16

func (v *PVUShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(*v))
}
func (v *PVUShort) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVUShort(data)
}

type PVInt int32

func (v *PVInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(*v))
}
func (v *PVInt) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVInt(data)
}

type PVUInt uint32

func (v *PVUInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(*v))
}
func (v *PVUInt) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVUInt(data)
}

type PVLong int64

func (v *PVLong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(*v))
}
func (v *PVLong) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVLong(data)
}

type PVULong uint64

func (v *PVULong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(*v))
}
func (v *PVULong) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVULong(data)
}

type PVFloat float32

func (v *PVFloat) PVEncode(s *EncoderState) error {
	return s.WriteUint32(math.Float32bits(*v))
}
func (v *PVFloat) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVFloat(math.Float32frombits(data))
}

type PVDouble float64

func (v *PVDouble) PVEncode(s *EncoderState) error {
	return s.WriteUint64(math.Float64bits(*v))
}
func (v *PVDouble) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVDouble(math.Float64frombits(data))
}

// Arrays

func valueToPVField(v reflect.Value) PVField {
	if v.CanInterface() {
		i := v.Interface()
		switch i.(type) {
		case PVField:
			return i
		case bool:
			return PVBoolean(i)
		case int8:
			return PVByte(i)
		case uint8:
			return PVUByte(i)
		case int16:
			return PVShort(i)
		case uint16:
			return PVUShort(i)
		case int32:
			return PVInt(i)
		case uint32:
			return PVUint(i)
		case int64:
			return PVLong(i)
		case uint64:
			return PVULong(i)
		case float32:
			return PVFloat(i)
		case float64:
			return PVDouble(i)
		case string:
			return PVString(i)
		}
	}
	if v.Kind() == reflect.Slice {
		return pvArray(v)
	}
	return nil
}

// TODO: Export this.

type pvArray reflect.Value

func (v pvArray) PVEncode(s *EncoderState) error {
	if err := PVSize(v.Len()).PVEncode(s); err != nil {
		return err
	}
	for i := 0; i < v.Len(); i++ {
		if err := valueToPVField(v.Index(i)).PVEncode(s); err != nil {
			return err
		}
	}
}
func (v pvArray) PVDecode(s *DecoderState) error {
	var s PVSize
	if err := s.PVDecode(s); err != nil {
		return err
	}
	if v.Cap() < s {
		v.Set(reflect.MakeSlice(v.Type(), s, s))
	}
	v.SetLen(s)
	for i := 0; i < s; i++ {
		if err := valueToPVField(v.Index(i)).PVDecode(s); err != nil {
			return err
		}
	}
	return nil
}

// TODO: Structure arrays have an extra boolean before each element to indicate if they are null.

// TODO: Fixed-size arrays that don't write a size.

// String types
type PVString string

func (v *PVString) PVEncode(s *EncoderState) error {
	if err := PVSize(len(v)).PVEncode(s); err != nil {
		return err
	}
	_, err := s.Buf.WriteString(string(*v))
	return err
}
func (v *PVString) PVDecode(s *DecoderState) error {
	var s PVSize
	if err := s.PVDecode(s); err != nil {
		return err
	}
	bytes := make([]byte, int(s))
	if _, err := io.ReadFull(s.Buf, bytes); err != nil {
		return err
	}
	*v = PVString(bytes)
	return nil
}

// Structure types

// TODO: Encoded as all of their fields in order.

// Union types

// TODO: Regular union is selector value encoded as size, followed by data

// TODO: Variant union is a field description, followed by data

// BitSet type

// Status type

const (
	PVStatus_OK      = PVStatus(0)
	PVStatus_WARNING = PVStatus(1)
	PVStatus_ERROR   = PVStatus(2)
	PVStatus_FATAL   = PVStatus(3)
)

type PVStatus struct {
	Type     PVByte
	Message  PVString
	CallTree PVString
}

func (v *PVStatus) PVEncode(s *EncoderState) error {
	if v.Type == PVStatus_OK && len(Message) == 0 && len(CallTree) == 0 {
		return s.Buf.WriteByte(byte(-1))
	}
	s.Buf.WriteByte(v.Type)
	if err := v.Message.PVEncode(s); err != nil {
		return err
	}
	return v.CallTree.PVEncode(s)
}
func (v *PVStatus) PVDecode(s *DecoderState) error {
	t, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	if t == -1 {
		v.Type = PVStatus_OK
		v.Message = ""
		v.CallTree = ""
		return nil
	}
	v.Type = PVByte(t)
	if err := v.Message.PVDecode(s); err != nil {
		return err
	}
	return v.CallTree.PVDecode(s)
}

// Introspection data

const (
	NULL_TYPE_CODE           = 0xFF
	ONLY_ID_TYPE_CODE        = 0xFE
	FULL_WITH_ID_TYPE_CODE   = 0xFD
	FULL_TAGGED_ID_TYPE_CODE = 0xFC
)

// TODO: Parse these
