package pvdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

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

	changedBitSet    PVBitSet
	useChangedBitSet bool
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
func (s *EncoderState) PushWriter(w Writer) func() {
	oldBuf := s.Buf
	s.Buf = w
	return func() {
		s.Buf = oldBuf
	}
}

type Reader interface {
	io.Reader
	io.ByteReader
}

type DecoderState struct {
	Buf       Reader
	ByteOrder binary.ByteOrder

	changedBitSet      PVBitSet
	useChangedBitSet   bool
	changedBitSetIndex int
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
func (s *DecoderState) PushReader(r Reader) func() {
	oldBuf := s.Buf
	s.Buf = r
	return func() { s.Buf = oldBuf }
}
func (s *DecoderState) pushChangedBitSet(bs PVBitSet) func() {
	oldCBS := s.changedBitSet
	oldUCBS := s.useChangedBitSet
	oldCBSI := s.changedBitSetIndex
	s.changedBitSet = bs
	s.useChangedBitSet = true
	s.changedBitSetIndex = 0
	return func() {
		s.changedBitSet = oldCBS
		s.useChangedBitSet = oldUCBS
		s.changedBitSetIndex = oldCBSI
	}
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
	if v < max8 {
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

func (v PVBoolean) PVEncode(s *EncoderState) error {
	if v {
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
func (PVBoolean) Field() (Field, error) {
	return Field{TypeCode: BOOLEAN}, nil
}

type PVByte int8

func (v PVByte) PVEncode(s *EncoderState) error {
	return s.Buf.WriteByte(byte(v))
}
func (v *PVByte) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = PVByte(data)
	return nil
}
func (PVByte) Field() (Field, error) {
	return Field{TypeCode: BYTE}, nil
}

type PVUByte uint8

func (v PVUByte) PVEncode(s *EncoderState) error {
	return s.Buf.WriteByte(byte(v))
}
func (v *PVUByte) PVDecode(s *DecoderState) error {
	data, err := s.Buf.ReadByte()
	if err != nil {
		return err
	}
	*v = PVUByte(data)
	return nil
}
func (PVUByte) Field() (Field, error) {
	return Field{TypeCode: UBYTE}, nil
}

type PVShort int16

func (v PVShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(v))
}
func (v *PVShort) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVShort(data)
	return nil
}
func (PVShort) Field() (Field, error) {
	return Field{TypeCode: SHORT}, nil
}

type PVUShort uint16

func (v PVUShort) PVEncode(s *EncoderState) error {
	return s.WriteUint16(uint16(v))
}
func (v *PVUShort) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint16()
	if err != nil {
		return err
	}
	*v = PVUShort(data)
	return nil
}
func (PVUShort) Field() (Field, error) {
	return Field{TypeCode: USHORT}, nil
}

type PVInt int32

func (v PVInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(v))
}
func (v *PVInt) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVInt(data)
	return nil
}
func (PVInt) Field() (Field, error) {
	return Field{TypeCode: INT}, nil
}

type PVUInt uint32

func (v PVUInt) PVEncode(s *EncoderState) error {
	return s.WriteUint32(uint32(v))
}
func (v *PVUInt) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVUInt(data)
	return nil
}
func (PVUInt) Field() (Field, error) {
	return Field{TypeCode: UINT}, nil
}

type PVLong int64

func (v PVLong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(v))
}
func (v *PVLong) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVLong(data)
	return nil
}
func (PVLong) Field() (Field, error) {
	return Field{TypeCode: LONG}, nil
}

type PVULong uint64

func (v PVULong) PVEncode(s *EncoderState) error {
	return s.WriteUint64(uint64(v))
}
func (v *PVULong) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVULong(data)
	return nil
}
func (PVULong) Field() (Field, error) {
	return Field{TypeCode: ULONG}, nil
}

type PVFloat float32

func (v PVFloat) PVEncode(s *EncoderState) error {
	return s.WriteUint32(math.Float32bits(float32(v)))
}
func (v *PVFloat) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint32()
	if err != nil {
		return err
	}
	*v = PVFloat(math.Float32frombits(data))
	return nil
}
func (PVFloat) Field() (Field, error) {
	return Field{TypeCode: FLOAT}, nil
}

type PVDouble float64

func (v PVDouble) PVEncode(s *EncoderState) error {
	return s.WriteUint64(math.Float64bits(float64(v)))
}
func (v *PVDouble) PVDecode(s *DecoderState) error {
	data, err := s.ReadUint64()
	if err != nil {
		return err
	}
	*v = PVDouble(math.Float64frombits(data))
	return nil
}
func (PVDouble) Field() (Field, error) {
	return Field{TypeCode: DOUBLE}, nil
}

// Arrays

type PVArray struct {
	fixed       bool
	alwaysShort bool
	// TODO: bounded-size arrays
	v reflect.Value
}

func NewPVFixedArray(slicePtr interface{}) PVArray {
	return PVArray{
		fixed: true,
		v:     reflect.ValueOf(slicePtr).Elem(),
	}
}

func (a PVArray) PVEncode(s *EncoderState) error {
	if s.useChangedBitSet {
		// Arrays do not contribute to the bitset.
		defer func() { s.useChangedBitSet = true }()
		s.useChangedBitSet = false
	}
	if !a.fixed {
		if a.alwaysShort {
			if err := PVUShort(a.v.Len()).PVEncode(s); err != nil {
				return err
			}
		} else {
			if err := PVSize(a.v.Len()).PVEncode(s); err != nil {
				return err
			}
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
	if s.useChangedBitSet {
		// Arrays do not contribute to the bitset.
		defer func() { s.useChangedBitSet = true }()
		s.useChangedBitSet = false
	}
	var size PVSize
	if a.fixed {
		size = PVSize(a.v.Len())
	} else {
		if a.alwaysShort {
			var sizeShort PVUShort
			if err := sizeShort.PVDecode(s); err != nil {
				return err
			}
			size = PVSize(sizeShort)
		} else {
			if err := size.PVDecode(s); err != nil {
				return err
			}
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
func (a PVArray) Field() (Field, error) {
	var prototype reflect.Value
	if a.v.Len() > 0 {
		prototype = a.v.Index(0).Addr()
	} else {
		prototype = reflect.New(a.v.Type().Elem())
	}
	f, err := valueToField(prototype)
	if err != nil {
		return Field{}, err
	}
	if a.fixed {
		f.TypeCode |= FIXED_ARRAY
		f.Size = PVSize(a.v.Len())
	} else {
		f.TypeCode |= VARIABLE_ARRAY
	}
	return f, nil
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

// TODO: Bounded string

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
func (v PVString) Field() (Field, error) {
	return Field{
		TypeCode: STRING,
		Size:     PVSize(len(v)),
	}, nil
}

type PVBoundedString struct {
	*PVString
	Bound PVSize
}

func (v PVBoundedString) PVEncode(s *EncoderState) error {
	if len(*v.PVString) > int(v.Bound) {
		return fmt.Errorf("string of %d bytes exceeds bound of %d bytes", len(*v.PVString), v.Bound)
	}
	return v.PVString.PVEncode(s)
}
func (v PVBoundedString) Field() (Field, error) {
	return Field{
		TypeCode: BOUNDED_STRING,
		Size:     v.Bound,
	}, nil
}

// Structure types

type PVStructure struct {
	ID string
	v  reflect.Value
}

// NewPVStructure creates a PVStructure with a given type ID from a pointer to a struct type.
func NewPVStructure(data interface{}, id string) PVStructure {
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		panic("data was not a struct")
	}
	return PVStructure{
		v: v,
	}
}

// TODO: Support bitfields for partial pack/unpack
func (v PVStructure) PVEncode(s *EncoderState) error {
	t := v.v.Type()
	for i := 0; i < v.v.NumField(); i++ {
		vf := v.v.Field(i)
		_, tags := parseTag(t.Field(i).Tag.Get("pvaccess"))
		if tags["omitifnil"] != "" && vf.Kind() == reflect.Ptr && (!vf.IsValid() || vf.IsNil()) {
			continue
		}
		item := vf.Addr()
		pvf := valueToPVField(item, tagsToOptions(tags)...)
		if pvf == nil {
			return fmt.Errorf("don't know how to encode %#v", item.Interface())
		}
		if s.useChangedBitSet {
			// TODO: Check if the field has actually changed.
			s.changedBitSet.Present = append(s.changedBitSet.Present, true)
		}
		if err := pvf.PVEncode(s); err != nil {
			return err
		}
		if _, ok := tags["breakonerror"]; ok {
			if pvf.(*PVStatus).Type > PVStatus_WARNING {
				return nil
			}
		}
	}
	return nil
}
func (v PVStructure) PVDecode(s *DecoderState) error {
	if !v.v.IsValid() {
		return errors.New("zero PVStructure is not usable")
	}
	// If the struct's bit itself is set, all the fields must be serialized
	// TODO: What about fields of child structs? Is this recursive?
	fullStruct := !s.useChangedBitSet || s.changedBitSet.Get(s.changedBitSetIndex)
	t := v.v.Type()
	for i := 0; i < v.v.NumField(); i++ {
		item := v.v.Field(i).Addr()
		_, tags := parseTag(t.Field(i).Tag.Get("pvaccess"))
		if s.useChangedBitSet {
			s.changedBitSetIndex++
		}
		pvf := valueToPVField(item, tagsToOptions(tags)...)
		if pvf == nil {
			return fmt.Errorf("don't know how to encode %#v", item.Interface())
		}
		_, isStruct := pvf.(PVStructure)
		if fullStruct || isStruct || s.changedBitSet.Get(s.changedBitSetIndex) {
			if err := pvf.PVDecode(s); err != nil {
				return err
			}
		}
		if _, ok := tags["breakonerror"]; ok {
			if item.Interface().(*PVStatus).Type > PVStatus_WARNING {
				return nil
			}
		}
	}
	return nil
}
func (v PVStructure) String() string {
	return fmt.Sprintf("%s%+v", v.ID, v.v.Interface())
}

func (v PVStructure) Field() (Field, error) {
	var fields []StructFieldDesc
	t := v.v.Type()
	for i := 0; i < v.v.NumField(); i++ {
		name, tags := parseTag(t.Field(i).Tag.Get("pvaccess"))
		if name == "" {
			name = t.Field(i).Name
		}
		vf := v.v.Field(i)
		if tags["omitifnil"] != "" && vf.Kind() == reflect.Ptr && (!vf.IsValid() || vf.IsNil()) {
			continue
		}
		f, err := valueToField(v.v.Field(i))
		if err != nil {
			return Field{}, fmt.Errorf("calling Field on %s: %v", name, err)
		}
		fields = append(fields, StructFieldDesc{
			Name:  name,
			Field: f,
		})
	}
	return Field{
		TypeCode:   STRUCT,
		StructType: PVString(v.ID),
		Fields:     fields,
	}, nil
}

func (v PVStructure) SubField(name string) PVField {
	t := v.v.Type()
	for i := 0; i < v.v.NumField(); i++ {
		got, _ := parseTag(t.Field(i).Tag.Get("pvaccess"))
		if got == "" {
			got = t.Field(i).Name
		}
		if got == name {
			return valueToPVField(v.v.Field(i).Addr())
		}
	}
	return nil
}

type PVStructureDiff struct {
	ChangedBitSet PVBitSet
	Value         interface{}
}

func (v PVStructureDiff) PVEncode(s *EncoderState) error {
	var buf bytes.Buffer
	if err := func() error {
		defer s.PushWriter(&buf)()
		s.changedBitSet = PVBitSet{Present: []bool{false}}
		s.useChangedBitSet = true
		return Encode(s, v.Value)
	}(); err != nil {
		return err
	}
	v.ChangedBitSet = s.changedBitSet
	if err := Encode(s, s.changedBitSet); err != nil {
		return err
	}
	if _, err := buf.WriteTo(s.Buf); err != nil {
		return err
	}
	return nil
}

func (v *PVStructureDiff) PVDecode(s *DecoderState) error {
	if err := Decode(s, &v.ChangedBitSet); err != nil {
		return err
	}
	defer s.pushChangedBitSet(v.ChangedBitSet)()
	return Decode(s, v.Value)
}

// Union types

// TODO: Regular union is selector value encoded as size, followed by data

// PVAny is a variant union, encoded as a field description, followed by data
type PVAny struct {
	Data PVField
}

func NewPVAny(data interface{}) PVAny {
	return PVAny{
		valueToPVField(reflect.ValueOf(data)),
	}
}

// PVEncode outputs a field description followed by the serialized field.
// As a special case, encoding a nil PVAny pointer will output zero bytes.
func (v *PVAny) PVEncode(s *EncoderState) error {
	if v == nil {
		return nil
	}
	if v.Data == nil {
		return Encode(s, &Field{TypeCode: NULL_TYPE_CODE})
	}
	f, err := valueToField(reflect.ValueOf(&v.Data))
	if err != nil {
		return err
	}
	return Encode(s, &f, v.Data)
}
func (v *PVAny) PVDecode(s *DecoderState) error {
	var f Field
	if err := Decode(s, &f); err != nil {
		return err
	}
	if f.TypeCode == NULL_TYPE_CODE {
		v.Data = nil
		return nil
	}
	data, err := f.createZero()
	if err != nil {
		return err
	}
	v.Data = data
	return Decode(s, v.Data)
}

// BitSet type
type PVBitSet struct {
	Present []bool
	// TODO: Consider using github.com/willf/bitset.BitSet
}

func NewBitSetWithBits(bits ...int) PVBitSet {
	max := -1
	for _, b := range bits {
		if b > max {
			max = b
		}
	}
	bs := PVBitSet{
		Present: make([]bool, max+1),
	}
	for _, b := range bits {
		bs.Present[b] = true
	}
	return bs
}

func (bs PVBitSet) Get(bit int) bool {
	if bit < len(bs.Present) {
		return bs.Present[bit]
	}
	return false
}

func (bs PVBitSet) PVEncode(s *EncoderState) error {
	size := PVSize((len(bs.Present) + 7) / 8)
	if err := Encode(s, &size); err != nil {
		return err
	}
	for i := uint(0); i < uint(size*8); i += 64 {
		// 64 bits are packed in a long, remainder are packed one at a time in a byte.
		if i+64 <= uint(size*8) {
			var out uint64
			for j := uint(0); j < 64 && (i+j) < uint(len(bs.Present)); j++ {
				if bs.Present[i+j] {
					out |= (1 << j)
				}
			}
			if err := Encode(s, &out); err != nil {
				return err
			}
		} else {
			for j := i; j < uint(len(bs.Present)); j += 8 {
				var out byte
				for k := uint(0); k < 8 && (j+k) < uint(len(bs.Present)); k++ {
					if bs.Present[j+k] {
						out |= (1 << k)
					}
				}
				if err := Encode(s, &out); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (bs *PVBitSet) PVDecode(s *DecoderState) error {
	bs.Present = nil

	var size PVSize
	if err := Decode(s, &size); err != nil {
		return err
	}

	maxBit := -1

	i := 0
	for ; i+7 < int(size); i += 8 {
		var in uint64
		if err := Decode(s, &in); err != nil {
			return err
		}
		for j := 0; j < 64; j++ {
			val := (in & 1) == 1
			if val {
				maxBit = i*8 + j
			}
			bs.Present = append(bs.Present, val)
			in >>= 1
		}
	}
	for ; i < int(size); i++ {
		var in byte
		if err := Decode(s, &in); err != nil {
			return err
		}
		for j := 0; j < 8; j++ {
			val := (in & 1) == 1
			if val {
				maxBit = i*8 + j
			}
			bs.Present = append(bs.Present, val)
			in >>= 1
		}
	}
	bs.Present = bs.Present[:maxBit+1]
	return nil
}

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
func (v PVStatus) Error() string {
	if v.Type == PVStatus_OK {
		return "OK"
	}
	return fmt.Sprintf("%d: %s", v.Type, v.Message)
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
	ID            PVUShort
	Tag           PVInt // FIXME: Figure out size of tag
	Size          PVSize
	StructType    PVString
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
	if f.TypeCode == NULL_TYPE_CODE {
		return nil
	}
	if f.TypeCode&0x10 == 0x10 || f.TypeCode == BOUNDED_STRING {
		if err := f.Size.PVEncode(s); err != nil {
			return err
		}
	}
	if f.TypeCode == STRUCT || f.TypeCode == UNION {
		if err := Encode(s, &f.StructType, f.Fields); err != nil {
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
	case NULL_TYPE_CODE:
		return nil
	case ONLY_ID_TYPE_CODE:
		// TODO: Look up in DecoderState
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
		if err := Decode(s, &f.StructType, &f.Fields); err != nil {
			return err
		}

	}
	// TODO: If f.HasID, save this Field in DecoderState so it can be reused.
	// TODO: Deserialize STRUCT_ARRAY and UNION_ARRAY types
	return nil
}

func (f Field) createZero() (PVField, error) {
	switch f.TypeCode {
	case NULL_TYPE_CODE:
		return nil, nil
	}
	// TODO: Create arrays
	if f.TypeCode&ARRAY_BITS == 0 {
		var prototype interface{}
		switch f.TypeCode {
		case BOOLEAN:
			prototype = PVBoolean(false)
		case BYTE:
			prototype = PVByte(0)
		case SHORT:
			prototype = PVShort(0)
		case INT:
			prototype = PVInt(0)
		case LONG:
			prototype = PVLong(0)
		case UBYTE:
			prototype = PVUByte(0)
		case USHORT:
			prototype = PVUShort(0)
		case UINT:
			prototype = PVUInt(0)
		case ULONG:
			prototype = PVULong(0)
		case FLOAT:
			prototype = PVFloat(0)
		case DOUBLE:
			prototype = PVDouble(0)
		case STRING:
			prototype = PVString("")
		case BOUNDED_STRING:
			var str PVString
			return &PVBoundedString{&str, f.Size}, nil
		}
		if prototype != nil {
			return reflect.New(reflect.TypeOf(prototype)).Interface().(PVField), nil
		}
	}
	if f.TypeCode == STRUCT {
		if f.StructType != "" {
			for _, t := range ntTypes {
				if string(f.StructType) == t.TypeID() {
					val := reflect.New(reflect.TypeOf(t)).Elem()
					return PVStructure{ID: string(f.StructType), v: val}, nil
				}
			}
		}
		// TODO: Support other NT types specially?
		var fields []reflect.StructField
		var zeros []PVField
		for _, field := range f.Fields {
			prototype, err := field.Field.createZero()
			if err != nil {
				return nil, err
			}
			zeros = append(zeros, prototype)
			name := field.Name
			if len(name) > 0 {
				name = strings.ToUpper(name[0:1]) + name[1:]
			}
			t := reflect.TypeOf(prototype)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			fields = append(fields, reflect.StructField{
				Name: name,
				Type: t,
				Tag:  reflect.StructTag("pvaccess:\"" + field.Name + "\""),
			})
		}
		val := reflect.New(reflect.StructOf(fields))
		for i, zero := range zeros {
			v := reflect.ValueOf(zero)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			val.Elem().Field(i).Set(v)
		}
		pvs := PVStructure{ID: string(f.StructType), v: val.Elem()}
		return pvs, nil
	}
	return nil, fmt.Errorf("don't know how to create zero value for %#v", f)
}
