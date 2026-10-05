package desktop

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeExportKeepsExistingFileOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.json")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeExportFile(path, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return errors.New("disk full")
	})
	if err == nil {
		t.Fatal("export must return the write failure")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "original" {
		t.Fatalf("failed export replaced existing file: contents=%s err=%v", contents, err)
	}
	if err := writeExportFile(path, func(w io.Writer) error {
		_, err := io.WriteString(w, "已保存")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(path)
	if err != nil || string(contents) != "已保存" {
		t.Fatalf("completed export not saved: contents=%s err=%v", contents, err)
	}
}
