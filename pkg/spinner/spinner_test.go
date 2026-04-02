package spinner

import "testing"

type fdTestWriter struct{}

func (fdTestWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (fdTestWriter) Fd() uintptr {
	return 42
}

func TestWriterPreservesFd(t *testing.T) {
	wrapped := Writer(fdTestWriter{})
	fdWriter, ok := wrapped.(interface{ Fd() uintptr })
	if !ok {
		t.Fatal("expected wrapped writer to expose Fd")
	}

	if got := fdWriter.Fd(); got != 42 {
		t.Fatalf("expected fd 42, got %d", got)
	}
}
