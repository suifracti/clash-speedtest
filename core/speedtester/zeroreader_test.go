package speedtester

import (
	"io"
	"testing"
)

func TestZeroReaderBasic(t *testing.T) {
	zr := NewZeroReader(100)
	if zr == nil {
		t.Fatal("expected non-nil ZeroReader")
	}
	if zr.RemainBytes() != 100 {
		t.Fatalf("expected 100 remain bytes, got %d", zr.RemainBytes())
	}
	if zr.WrittenBytes() != 0 {
		t.Fatalf("expected 0 written bytes, got %d", zr.WrittenBytes())
	}

	buf := make([]byte, 40)
	n, err := zr.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error on read: %v", err)
	}
	if n != 40 {
		t.Fatalf("expected 40 bytes read, got %d", n)
	}
	if zr.RemainBytes() != 60 {
		t.Fatalf("expected 60 remain bytes, got %d", zr.RemainBytes())
	}
	if zr.WrittenBytes() != 40 {
		t.Fatalf("expected 40 written bytes, got %d", zr.WrittenBytes())
	}
	for i, b := range buf {
		if b != 0 {
			t.Fatalf("expected byte at index %d to be 0, got %d", i, b)
		}
	}

	// Read remaining 60 bytes
	buf2 := make([]byte, 100)
	n, err = zr.Read(buf2)
	if err != nil {
		t.Fatalf("unexpected error on read: %v", err)
	}
	if n != 60 {
		t.Fatalf("expected 60 bytes read, got %d", n)
	}
	if zr.RemainBytes() != 0 {
		t.Fatalf("expected 0 remain bytes, got %d", zr.RemainBytes())
	}
	if zr.WrittenBytes() != 100 {
		t.Fatalf("expected 100 written bytes, got %d", zr.WrittenBytes())
	}

	// Next read should return EOF
	n, err = zr.Read(buf2)
	if err != io.EOF {
		t.Fatalf("expected EOF on exhausted reader, got err=%v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes, got %d", n)
	}
}

func TestZeroReaderNegativeSizeClamping(t *testing.T) {
	zr := NewZeroReader(-50)
	if zr == nil {
		t.Fatal("expected non-nil ZeroReader")
	}
	if zr.RemainBytes() != 0 {
		t.Fatalf("expected 0 remain bytes for negative size, got %d", zr.RemainBytes())
	}
	buf := make([]byte, 10)
	n, err := zr.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF for zero size, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes read, got %d", n)
	}
}

func TestZeroReaderNilReceiver(t *testing.T) {
	var zr *ZeroReader
	if zr.RemainBytes() != 0 {
		t.Fatalf("expected 0 remain bytes for nil reader, got %d", zr.RemainBytes())
	}
	if zr.WrittenBytes() != 0 {
		t.Fatalf("expected 0 written bytes for nil reader, got %d", zr.WrittenBytes())
	}
	buf := make([]byte, 10)
	n, err := zr.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF on nil reader, got err=%v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 bytes, got %d", n)
	}
}
