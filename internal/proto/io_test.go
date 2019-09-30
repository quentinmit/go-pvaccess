package proto

import (
	"bytes"
	"testing"
)

func TestAligningWriter(t *testing.T) {
	var buf bytes.Buffer
	a := NewAligningWriter(&buf)
	checkBytes := func(want int) {
		t.Helper()
		if got := buf.Len(); got != want {
			t.Errorf("wrong number of bytes written: got %d, want %d", got, want)
		}
	}

	checkBytes(0)

	if n, err := a.Write([]byte{1, 2, 3}); n != 3 || err != nil {
		t.Errorf("failed to write: got %d, %v, want 3, nil", n, err)
	}

	checkBytes(3)

	if n, err := a.Align(); n != 5 || err != nil {
		t.Errorf("failed to align: got %d, %v, want 5, nul", n, err)
	}

	checkBytes(8)
}
