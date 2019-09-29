package pvdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestField(t *testing.T) {
	tests := []struct {
		in   interface{}
		want []byte
	}{
		{true, []byte{0}},
		{int8(1), []byte{0x20}},
		{uint8(1), []byte{0x24}},
		{int16(1), []byte{0x21}},
		{uint16(1), []byte{0x25}},
		{int32(1), []byte{0x22}},
		{uint32(1), []byte{0x26}},
		{int64(1), []byte{0x23}},
		{uint64(1), []byte{0x27}},
		{float32(1), []byte{0x42}},
		{float64(1), []byte{0x43}},
		{[]uint8{}, []byte{0x2c}},
		{[4]uint8{1, 2, 3, 4}, []byte{0x3c, 4}},
	}
	for _, test := range tests {
		name := fmt.Sprintf("%T: %#v", test.in, test.in)
		t.Run(name, func(t *testing.T) {
			// Make a copy on the stack
			in := reflect.New(reflect.TypeOf(test.in))
			in.Elem().Set(reflect.ValueOf(test.in))
			f, err := valueToField(in)

			if err != nil {
				t.Errorf("failed to convert to Field: %v", err)
			}

			var buf bytes.Buffer
			es := &EncoderState{
				Buf:       &buf,
				ByteOrder: binary.BigEndian,
			}
			if err := f.PVEncode(es); err != nil {
				t.Errorf("unexpected encode error: %v", err)
			}
			if diff := cmp.Diff(buf.Bytes(), test.want); diff != "" {
				t.Errorf("wrong FieldDesc. got(-)/want(+)\n%s", diff)
			}
		})
	}
}
