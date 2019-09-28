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
	tests := []struct {
		in             PVSize
		wantBE, wantLE []byte
	}{
		{-1, []byte{255}, nil},
		{0, []byte{0}, nil},
		{254, []byte{254, 0, 0, 0, 254}, []byte{254, 254, 0, 0, 0}},
		{0x7fffffff, []byte{254, 0x7f, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}, []byte{254, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff, 0x7f, 0, 0, 0, 0}},
	}
	for _, test := range tests {
		testOneValue(t, fmt.Sprintf("PVSize(%d)", test.in), &test.in, test.wantBE, test.wantLE)
	}
}

func testOneValue(t *testing.T, name string, in interface{}, wantBE, wantLE []byte) {
	t.Helper()

	if name == "" {
		name = fmt.Sprintf("%T: %#v", in, in)
	}
	t.Run(name, func(t *testing.T) {
		pvf := valueToPVField(reflect.ValueOf(in))
		if pvf == nil && wantBE != nil {
			t.Fatal("failed to convert to PVField; expected PVField implementation")
		}
		if pvf != nil && wantBE == nil {
			t.Fatalf("got instance of %T, expected conversion failure", pvf)
		}
		if wantLE == nil {
			wantLE = wantBE
		}
		for _, byteOrder := range []struct {
			byteOrder binary.ByteOrder
			want      []byte
		}{
			{binary.BigEndian, wantBE},
			{binary.LittleEndian, wantLE},
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
