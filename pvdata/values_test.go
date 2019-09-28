package pvdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
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

func TestPVEncode(t *testing.T) {
	tests := []struct {
		in             interface{}
		wantBE, wantLE []byte
	}{
		{PVSize(-1), []byte{255}, nil},
		{PVSize(0), []byte{0}, nil},
		{PVSize(254), []byte{254, 0, 0, 0, 254}, []byte{254, 254, 0, 0, 0}},
		{PVSize(0x7fffffff), []byte{254, 0x7f, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}, []byte{254, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0}},
		{PVBoolean(true), []byte{1}, nil},
		{PVBoolean(false), []byte{0}, nil},
		{true, []byte{1}, nil},
		{false, []byte{0}, nil},
	}
	for _, test := range tests {
		name := fmt.Sprintf("%T: %#v", test.in, test.in)
		t.Run(name, func(t *testing.T) {
			// Make a copy on the stack
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
					s := &EncoderState{
						Buf:       &buf,
						ByteOrder: byteOrder.byteOrder,
					}
					if err := pvf.PVEncode(s); err != nil {
						t.Errorf("unexpected encode error: %v", err)
					}
					if diff := cmp.Diff(buf.Bytes(), byteOrder.want); diff != "" {
						t.Errorf("got(-)/want(+)\n%s", diff)
					}
				})
			}
		})
	}
}
