package pvaccess

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"os"
	"testing"

	"github.com/quentinmit/go-pvaccess/internal/proto"
	"github.com/quentinmit/go-pvaccess/pvdata"
)

func TestConnectionBanner(t *testing.T) {
	// TODO: Table-driven test with various input packets
	input := []byte{}
	var buf bytes.Buffer
	writer := proto.NewAligningWriter(&buf)
	c := &connection{
		direction: proto.FLAG_FROM_SERVER,
		writer:    writer,
		log:       log.New(os.Stderr, "", log.LstdFlags),
		encoderState: &pvdata.EncoderState{
			Buf:       bufio.NewWriter(writer),
			ByteOrder: binary.LittleEndian,
		},
		decoderState: &pvdata.DecoderState{
			Buf: bytes.NewReader(input),
		},
	}
	if err := c.handleServer(); err != io.EOF {
		t.Errorf("handleServer failed: %v", err)
	}
	t.Logf("received handshake: %x", buf.Bytes())
}
