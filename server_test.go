package pvaccess

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type writeFlusher interface {
	io.Writer
	Flush() error
}

type readWriter struct {
	reader io.Reader
	writer writeFlusher
}

func (rw readWriter) Read(data []byte) (int, error) {
	return rw.reader.Read(data)
}
func (rw readWriter) Write(data []byte) (int, error) {
	return rw.writer.Write(data)
}
func (rw readWriter) Flush() error {
	return rw.writer.Flush()
}

func TestConnectionBanner(t *testing.T) {
	// TODO: Table-driven test with various input packets
	input := []byte{}
	var buf bytes.Buffer
	conn := &readWriter{
		bytes.NewReader(input),
		bufio.NewWriter(&buf),
	}
	c := (&Server{}).newConn(conn)
	if err := c.serve(); err != io.EOF {
		t.Errorf("serve failed: %v", err)
	}
	conn.Flush()
	// SET_BYTE_ORDER followed by CONNECTION_VALIDATION_REQUEST
	want := []byte{0xca, 0x02, 0x41, 0x02, 0x00, 0x00, 0x00, 0x00, 0xca, 0x02, 0x40, 0x01, 0x11, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xff, 0x7f, 0x01, 0x09, 0x61, 0x6e, 0x6f, 0x6e, 0x79, 0x6d, 0x6f, 0x75, 0x73}
	if diff := cmp.Diff(buf.Bytes(), want); diff != "" {
		t.Errorf("wrong handshake: got(-)/want(+)\n%s", diff)
	}
}
