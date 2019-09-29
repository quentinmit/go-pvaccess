package pvaccess

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

func TestConnectionBanner(t *testing.T) {
	// TODO: Table-driven test with various input packets
	input := []byte{}
	var buf bytes.Buffer
	c := &connection{
		direction: proto.FLAG_FROM_SERVER,
		encoderState: &pvdata.EncoderState{
			Buf:       &buf,
			ByteOrder: binary.LittleEndian,
		},
		decoderState: &pvdata.DecoderState{
			Buf: bytes.NewReader(input),
		},
	}
	if err := c.handleServer(); err != io.EOF {
		t.Errorf("handleServer failed: %v", err)
	}
	t.Logf("received handshake: %v", buf.Bytes())
}
