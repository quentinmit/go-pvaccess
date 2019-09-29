package pvdata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"

	"github.com/google/go-cmp/cmp"
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

func (s *EncoderState) WriteUint16(v uint16) error {
	bytes := make([]byte, 2)
	s.ByteOrder.PutUint16(bytes, v)
	return check(s.Buf.Write(bytes))
}
func (s *EncoderState) WriteUint32(v uint32) error {
	bytes := make([]byte, 4)
	s.ByteOrder.PutUint32(bytes, v)
	return check(s.Buf.Write(bytes))
}
func (s *EncoderState) WriteUint64(v uint64) error {
	bytes := make([]byte, 8)
	s.ByteOrder.PutUint64(bytes, v)
	return check(s.Buf.Write(bytes))
}

type Reader interface {
	io.Reader
	io.ByteReader
}

type DecoderState struct {
	Buf       Reader
	ByteOrder binary.ByteOrder
}

func (s *DecoderState) ReadUint16() (uint16, error) {
	bytes := make([]byte, 2)
	if _, err := io.ReadFull(s.Buf, bytes); err != nil {
		return 0, err
	}
	return s.ByteOrder.Uint16(bytes), nil
}
func (s *DecoderState) ReadUint32() (uint32, error) {
	bytes := make([]byte, 4)
	if _, err := io.ReadFull(s.Buf, bytes); err != nil {
		return 0, err
	}
	return s.ByteOrder.Uint32(bytes), nil
}
func (s *DecoderState) ReadUint64() (uint64, error) {
	bytes := make([]byte, 8)
	if _, err := io.ReadFull(s.Buf, bytes); err != nil {
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

func (v PVSize) PVEncode(s *EncoderState) error {
	if v < 0 {
		return s.Buf.WriteByte(255)
	}
	if v < 8 {
		return s.Buf.WriteByte(byte(v))
	}
	if v < max32 {
		out := make([]byte, 5)
		out[0] = max8
		s.ByteOrder.PutUint32(out[1:], uint32(v))
		return check(s.Buf.Write(out))
	}
	out := make([]byte, 1+4+8)
	out[0] = max8
	s.ByteOrder.PutUint32(out[1:5], max32)
	s.ByteOrder.PutUint64(out[5:], uint64(v))
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
	return nil
}
func (PVBoolean) Field() Field {
	return Field{TypeCode: BOOLEAN}
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
	return nil
}
func (PVByte) Field() Field {
	return Field{TypeCode: BYTE}
}

type PVUByte uint8

func (v *PVUByte) PVEncode(s *EncoderState) error {
	return s.Buf.WriteByte(byte(*v))
}
func (v *PVUByte) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = PVUByte(data)
	return nil
}
func (PVUByte) Field() Field {
	return Field{TypeCode: UBYTE}
}

type PVShort int16

func (v *PVShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(*v))
}
func (v *PVShort) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVShort(data)
	return nil
}
func (PVShort) Field() Field {
	return Field{TypeCode: SHORT}
}

type PVUShort uint16

func (v *PVUShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(*v))
}
func (v *PVUShort) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVUShort(data)
	return nil
}
func (PVUShort) Field() Field {
	return Field{TypeCode: USHORT}
}

type PVInt int32

func (v *PVInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(*v))
}
func (v *PVInt) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVInt(data)
	return nil
}
func (PVInt) Field() Field {
	return Field{TypeCode: INT}
}

type PVUInt uint32

func (v *PVUInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(*v))
}
func (v *PVUInt) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVUInt(data)
	return nil
}
func (PVUInt) Field() Field {
	return Field{TypeCode: UINT}
}

type PVLong int64

func (v *PVLong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(*v))
}
func (v *PVLong) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVLong(data)
	return nil
}
func (PVLong) Field() Field {
	return Field{TypeCode: LONG}
}

type PVULong uint64

func (v *PVULong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(*v))
}
func (v *PVULong) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVULong(data)
	return nil
}
func (PVULong) Field() Field {
	return Field{TypeCode: ULONG}
}

type PVFloat float32

func (v *PVFloat) PVEncode(s *EncoderState) error {
	return s.WriteUint32(math.Float32bits(float32(*v)))
}
func (v *PVFloat) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVFloat(math.Float32frombits(data))
	return nil
}
func (PVFloat) Field() Field {
	return Field{TypeCode: FLOAT}
}

type PVDouble float64

func (v *PVDouble) PVEncode(s *EncoderState) error {
	return s.WriteUint64(math.Float64bits(float64(*v)))
}
func (v *PVDouble) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVDouble(math.Float64frombits(data))
	return nil
}
func (PVDouble) Field() Field {
	return Field{TypeCode: DOUBLE}
}

// Arrays

type PVArray struct {
	fixed bool
	// TODO: bounded-size arrays
	v reflect.Value
}

func NewPVFixedArray(slicePtr interface{}) PVArray {
	return PVArray{
		true,
		reflect.ValueOf(slicePtr).Elem(),
	}
}

func (a PVArray) PVEncode(s *EncoderState) error {
	if !a.fixed {
		if err := PVSize(a.v.Len()).PVEncode(s); err != nil {
			return err
		}
	}
	for i := 0; i < a.v.Len(); i++ {
		item := a.v.Index(i).Addr()
		pvf := valueToPVField(item)
		if pvf == nil {
			return fmt.Errorf("don't know how to encode %#v", item.Interface())
		}
		if err := pvf.PVEncode(s); err != nil {
			return err
		}
	}
	return nil
}
func (a PVArray) PVDecode(s *DecoderState) error {
	if !a.v.IsValid() {
		return errors.New("zero PVArray is not usable")
	}
	var size PVSize
	if a.fixed {
		size = PVSize(a.v.Len())
	} else {
		if err := size.PVDecode(s); err != nil {
			return err
		}
		if a.v.Cap() < int(size) {
			a.v.Set(reflect.MakeSlice(a.v.Type(), int(size), int(size)))
		}
		a.v.SetLen(int(size))
	}
	for i := 0; i < int(size); i++ {
		item := a.v.Index(i).Addr()
		pvf := valueToPVField(item)
		if pvf == nil {
			return fmt.Errorf("don't know how to decode %#v", item.Interface())
		}
		if err := pvf.PVDecode(s); err != nil {
			return err
		}
	}
	return nil
}
func (a PVArray) Field() Field {
	var prototype reflect.Value
	if a.v.Len() > 0 {
		prototype = a.v.Index(0).Addr()
	} else {
		prototype = reflect.New(a.v.Type().Elem())
	}
	// TODO: Don't ignore the error
	f, _ := valueToField(prototype)
	if a.fixed {
		f.TypeCode |= FIXED_ARRAY
		f.Size = PVSize(a.v.Len())
	} else {
		f.TypeCode |= VARIABLE_ARRAY
	}
	return f
}
func (a PVArray) Equal(b PVArray) bool {
	if a.fixed == b.fixed && a.v.IsValid() && b.v.IsValid() {
		return cmp.Equal(a.v.Interface(), b.v.Interface())
	}
	return false
}

// TODO: Structure arrays have an extra boolean before each element to indicate if they are null.

// TODO: Fixed-size arrays that don't write a size.

// String types
type PVString string

func (v PVString) PVEncode(s *EncoderState) error {
	if err := PVSize(len(v)).PVEncode(s); err != nil {
		return err
	}
	_, err := s.Buf.WriteString(string(v))
	return err
}
func (v *PVString) PVDecode(s *DecoderState) error {
	var size PVSize
	if err := size.PVDecode(s); err != nil {
		return err
	}
	bytes := make([]byte, int(size))
	if _, err := io.ReadFull(s.Buf, bytes); err != nil {
		return err
	}
	*v = PVString(bytes)
	return nil
}

// Structure types

type PVStructure struct {
	v reflect.Value
}

// TODO: Support bitfields for partial pack/unpack
func (v PVStructure) PVEncode(s *EncoderState) error {
	for i := 0; i < v.v.NumField(); i++ {
		item := v.v.Field(i).Addr()
		pvf := valueToPVField(item)
		if pvf == nil {
			return fmt.Errorf("don't know how to encode %#v", item.Interface())
		}
		if err := pvf.PVEncode(s); err != nil {
			return err
		}
	}
	return nil
}
func (v PVStructure) PVDecode(s *DecoderState) error {
	if !v.v.IsValid() {
		return errors.New("zero PVStructure is not usable")
	}
	for i := 0; i < v.v.NumField(); i++ {
		item := v.v.Field(i).Addr()
		pvf := valueToPVField(item)
		if pvf == nil {
			return fmt.Errorf("don't know how to decode %#v", item.Interface())
		}
		if err := pvf.PVDecode(s); err != nil {
			return err
		}
	}
	return nil
}

// Union types

// TODO: Regular union is selector value encoded as size, followed by data

// TODO: Variant union is a field description, followed by data

type PVAny struct {
	Field Field
	Data  PVField
}

// BitSet type

// Status type

const (
	PVStatus_OK      = PVByte(0)
	PVStatus_WARNING = PVByte(1)
	PVStatus_ERROR   = PVByte(2)
	PVStatus_FATAL   = PVByte(3)
)

type PVStatus struct {
	Type     PVByte
	Message  PVString
	CallTree PVString
}

func (v *PVStatus) PVEncode(s *EncoderState) error {
	if v.Type == PVStatus_OK && len(v.Message) == 0 && len(v.CallTree) == 0 {
		return s.Buf.WriteByte(0xFF)
	}
	s.Buf.WriteByte(byte(v.Type))
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
	if t == 0xFF {
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

type StructFieldDesc struct {
	Name  string
	Field Field
}

type Field struct {
	// TypeCode represents the real FieldDesc, or NULL_TYPE_CODE
	// The other special codes will be inferred from HasID, HasTag
	TypeCode      byte
	HasID, HasTag bool
	ID            PVString
	Tag           PVInt // FIXME: Figure out size of tag
	Size          PVSize
	Fields        []StructFieldDesc
}

const (
	BOOLEAN = 0x00
	BYTE    = 0x20
	SHORT   = 0x21
	INT     = 0x22
	LONG    = 0x23
	UBYTE   = 0x24
	USHORT  = 0x25
	UINT    = 0x26
	ULONG   = 0x27
	FLOAT   = 0x42
	DOUBLE  = 0x43
	STRING  = 0x60

	ARRAY_BITS     = 0x18
	VARIABLE_ARRAY = 0x08
	BOUNDED_ARRAY  = 0x10
	FIXED_ARRAY    = 0x18
	BOUNDED_STRING = 0x86
	STRUCT         = 0x80
	UNION          = 0x81
	STRUCT_ARRAY   = STRUCT | VARIABLE_ARRAY
	UNION_ARRAY    = UNION | VARIABLE_ARRAY
)

func (f *Field) PVEncode(s *EncoderState) error {
	if f.HasID && f.HasTag {
		if err := s.Buf.WriteByte(FULL_TAGGED_ID_TYPE_CODE); err != nil {
			return err
		}
		if err := f.ID.PVEncode(s); err != nil {
			return err
		}
		if err := f.Tag.PVEncode(s); err != nil {
			return err
		}
	} else if f.HasID {
		if f.TypeCode == NULL_TYPE_CODE {
			if err := s.Buf.WriteByte(ONLY_ID_TYPE_CODE); err != nil {
				return err
			}
			return f.ID.PVEncode(s)
		}
		s.Buf.WriteByte(FULL_WITH_ID_TYPE_CODE)
		if err := f.ID.PVEncode(s); err != nil {
			return err
		}
	}
	if err := s.Buf.WriteByte(f.TypeCode); err != nil {
		return err
	}
	if f.TypeCode&0x10 == 0x10 || f.TypeCode == BOUNDED_STRING {
		if err := f.Size.PVEncode(s); err != nil {
			return err
		}
	}
	if f.TypeCode == STRUCT || f.TypeCode == UNION {
		// TODO: Is this the same as the top-level ID?
		if err := encode(s, f.ID, f.Fields); err != nil {
			return err
		}
	}
	// TODO: Serialize STRUCT_ARRAY and UNION_ARRAY types
	return nil
}

func (f *Field) PVDecode(s *DecoderState) error {
	typeCode, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	f.TypeCode = typeCode
	switch f.TypeCode {
	case ONLY_ID_TYPE_CODE:
		f.HasID = true
		f.TypeCode = NULL_TYPE_CODE
		return nil
	case FULL_WITH_ID_TYPE_CODE:
		f.HasID = true
	case FULL_TAGGED_ID_TYPE_CODE:
		f.HasID = true
		f.HasTag = true
	}
	if f.HasID {
		if err := f.ID.PVDecode(s); err != nil {
			return err
		}
	}
	if f.HasTag {
		if err := f.Tag.PVDecode(s); err != nil {
			return err
		}
	}
	switch f.TypeCode {
	case FULL_WITH_ID_TYPE_CODE, FULL_TAGGED_ID_TYPE_CODE:
		typeCode, err = s.Buf.ReadByte()
		if err != nil {
			return err
		}
		f.TypeCode = typeCode
	}
	if f.TypeCode&0x10 == 0x10 || f.TypeCode == BOUNDED_STRING {
		if err := f.Size.PVDecode(s); err != nil {
			return err
		}
	}
	if f.TypeCode == STRUCT || f.TypeCode == UNION {
		// TODO: Is this the same as the top-level ID?
		if err := decode(s, f.ID, f.Fields); err != nil {
			return err
		}

	}
	// TODO: Deserialize STRUCT_ARRAY and UNION_ARRAY types
	return nil
}
