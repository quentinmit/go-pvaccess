package proto

import "io"

const ALIGNMENT = 8

type AligningWriter struct {
	bytesWritten int64
	out          io.Writer
}

func NewAligningWriter(out io.Writer) *AligningWriter {
	return &AligningWriter{out: out}
}

func (w *AligningWriter) Write(p []byte) (n int, err error) {
	n, err = w.out.Write(p)
	w.bytesWritten += int64(n)
	return
}

func (w *AligningWriter) Align() (int, error) {
	padding := w.bytesWritten % ALIGNMENT
	if padding != 0 {
		return w.Write(make([]byte, (ALIGNMENT - padding)))
	}
	return 0, nil
}
