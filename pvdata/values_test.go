package pvdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPVSize(t *testing.T) {
	s := PVSize(0)
	var buf bytes.Buffer
	pvf := PVField(&s)
	if err := pvf.PVEncode(&EncoderState{
		Buf:       &buf,
		ByteOrder: binary.BigEndian,
	}); err != nil {
		t.Error(err)
	}
	bytes := buf.Bytes()
	if len(bytes) != 1 || bytes[0] != 0 {
		t.Errorf("bytes = %v, want [0]", bytes)
	}
}

var string254 = strings.Repeat("3", 254)

func TestRoundTrip(t *testing.T) {
	anyTargetInt := PVShort(12)
	exampleStruct := struct {
		Code    byte
		Message string
	}{15, "yes"}
	tests := []struct {
		in             interface{}
		wantBE, wantLE []byte
	}{
		{PVSize(-1), []byte{255}, nil},
		{PVSize(0), []byte{0}, nil},
		{PVSize(253), []byte{253}, nil},
		{PVSize(254), []byte{254, 0, 0, 0, 254}, []byte{254, 254, 0, 0, 0}},
		{PVSize(0x7fffffff), []byte{254, 0x7f, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}, []byte{254, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0}},
		{PVBoolean(true), []byte{1}, nil},
		{PVBoolean(false), []byte{0}, nil},
		{true, []byte{1}, nil},
		{false, []byte{0}, nil},
		{PVByte(0), []byte{0}, nil},
		{PVByte(-1), []byte{0xFF}, nil},
		{int8(127), []byte{0x7F}, nil},
		{PVUByte(0), []byte{0}, nil},
		{PVUByte(129), []byte{129}, nil},
		{byte(13), []byte{13}, nil},
		{PVShort(256), []byte{1, 0}, []byte{}},
		{PVShort(-1), []byte{0xff, 0xff}, []byte{}},
		{int16(32), []byte{0, 32}, []byte{}},
		{PVUShort(32768), []byte{0x80, 0x00}, []byte{}},
		{uint16(32768), []byte{0x80, 0x00}, []byte{}},
		{PVInt(65536), []byte{0, 1, 0, 0}, []byte{}},
		{PVInt(-1), []byte{0xff, 0xff, 0xff, 0xff}, []byte{}},
		{int32(32), []byte{0, 0, 0, 32}, []byte{}},
		{PVUInt(0x80000000), []byte{0x80, 0, 0, 0}, []byte{}},
		{uint32(1), []byte{0, 0, 0, 1}, []byte{}},
		{PVLong(-1), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, nil},
		{int64(1), []byte{0, 0, 0, 0, 0, 0, 0, 1}, []byte{}},
		{PVULong(0x8000000000000000), []byte{0x80, 0, 0, 0, 0, 0, 0, 0}, []byte{}},
		{uint64(13), []byte{0, 0, 0, 0, 0, 0, 0, 13}, []byte{}},
		{float32(85.125), []byte{0x42, 0xAA, 0x40, 0x00}, []byte{}},
		{float64(85.125), []byte{0x40, 0x55, 0x48, 0, 0, 0, 0, 0}, []byte{}},
		{[]PVBoolean{true, false, false}, []byte{3, 1, 0, 0}, nil},
		{[3]PVBoolean{true, false, false}, []byte{1, 0, 0}, nil},
		{NewPVFixedArray(&[]bool{false, true, true}), []byte{0, 1, 1}, nil},
		{PVString("33"), []byte{2, 0x33, 0x33}, nil},
		{string254, append([]byte{254, 0, 0, 0, 254}, []byte(string254)...), append([]byte{254, 254, 0, 0, 0}, []byte(string254)...)},
		{[]string{"1", "22", "333"}, []byte{3, 1, 0x31, 2, 0x32, 0x32, 3, 0x33, 0x33, 0x33}, nil},
		{[]PVString{"1", "22", "333"}, []byte{3, 1, 0x31, 2, 0x32, 0x32, 3, 0x33, 0x33, 0x33}, nil},
		{PVStatus{PVStatus_OK, "", ""}, []byte{0xFF}, nil},
		{PVStatus{PVStatus_FATAL, "3", "2"}, []byte{3, 1, 0x33, 1, 0x32}, nil},
		{exampleStruct, []byte{15, 3, 'y', 'e', 's'}, nil},
		{PVAny{&anyTargetInt}, []byte{0x21, 0, 12}, []byte{0x21, 12, 0}},
		{PVAny{nil}, []byte{0xff}, nil},
		{struct {
			A bool
			B []bool
		}{true, []bool{true}}, []byte{0x01, 0x01, 0x01}, nil},
		{struct {
			A bool
			B []bool `pvaccess:",short"`
		}{true, []bool{true}}, []byte{0x01, 0x00, 0x01, 0x01}, []byte{0x01, 0x01, 0x00, 0x01}},
		{PVBitSet{[]bool{}}, []byte{0}, nil},
		{PVBitSet{[]bool{true}}, []byte{1, 1}, nil},
		{PVBitSet{[]bool{false, true}}, []byte{1, 2}, nil},
		{NewBitSetWithBits(7), []byte{1, 0x80}, nil},
		{NewBitSetWithBits(8), []byte{2, 0, 1}, nil},
		{NewBitSetWithBits(15), []byte{2, 0, 0x80}, nil},
		{NewBitSetWithBits(55), []byte{7, 0, 0, 0, 0, 0, 0, 0x80}, nil},
		{NewBitSetWithBits(56), []byte{8, 0, 0, 0, 0, 0, 0, 0, 1}, nil},
		{NewBitSetWithBits(63), []byte{8, 0, 0, 0, 0, 0, 0, 0, 0x80}, nil},
		{NewBitSetWithBits(64), []byte{9, 0, 0, 0, 0, 0, 0, 0, 0, 1}, nil},
		{NewBitSetWithBits(65), []byte{9, 0, 0, 0, 0, 0, 0, 0, 0, 2}, nil},
		{NewBitSetWithBits(0, 1, 2, 4), []byte{1, 0x17}, nil},
		{NewBitSetWithBits(0, 1, 2, 4, 8), []byte{2, 0x17, 0x01}, nil},
	}
	for _, test := range tests {
		name := fmt.Sprintf("%T/%#v", test.in, test.in)
		t.Run(name, func(t *testing.T) {
			// Make a copy on the heap
			in := reflect.New(reflect.TypeOf(test.in))
			in.Elem().Set(reflect.ValueOf(test.in))
			pvf := valueToPVField(in)
			if pvf == nil && test.wantBE != nil {
				t.Fatal("failed to convert to PVField; expected PVField implementation")
			}
			if pvf != nil && test.wantBE == nil {
				t.Fatalf("got instance of %T, expected conversion failure", pvf)
			}
			if test.wantLE == nil {
				test.wantLE = test.wantBE
			} else if len(test.wantLE) == 0 {
				test.wantLE = make([]byte, len(test.wantBE))
				for i := 0; i < len(test.wantBE); i++ {
					test.wantLE[i] = test.wantBE[len(test.wantBE)-1-i]
				}
			}
			for _, byteOrder := range []struct {
				byteOrder binary.ByteOrder
				want      []byte
			}{
				{binary.BigEndian, test.wantBE},
				{binary.LittleEndian, test.wantLE},
			} {
				t.Run(byteOrder.byteOrder.String(), func(t *testing.T) {
					var buf bytes.Buffer
					es := &EncoderState{
						Buf:       &buf,
						ByteOrder: byteOrder.byteOrder,
					}
					if err := pvf.PVEncode(es); err != nil {
						t.Errorf("unexpected encode error: %v", err)
					}
					if diff := cmp.Diff(buf.Bytes(), byteOrder.want); diff != "" {
						t.Errorf("encode failed. got(-)/want(+)\n%s", diff)
					}

					ds := &DecoderState{
						Buf:       bytes.NewReader(byteOrder.want),
						ByteOrder: byteOrder.byteOrder,
					}
					out := reflect.New(reflect.TypeOf(test.in))
					opvf := valueToPVField(out)
					out = out.Elem()
					if in, ok := test.in.(PVArray); ok {
						pva := PVArray{in.fixed, in.alwaysShort, reflect.MakeSlice(in.v.Type(), in.v.Len(), in.v.Len())}
						out = reflect.ValueOf(pva)
						opvf = PVField(pva)
					}
					if err := opvf.PVDecode(ds); err != nil {
						t.Errorf("unexpected decode error: %v", err)
					}
					if diff := cmp.Diff(out.Interface(), test.in); diff != "" {
						t.Errorf("decode failed. got(-)/want(+)\n%s", diff)
					}
				})
			}
		})
	}
}
