package nas

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDlsPackage(t *testing.T) {
	dir := t.TempDir()
	old := packageDir
	packageDir = dir
	defer func() { packageDir = old }()
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if got := dlsPackage("test", f); string(got) != "NONE" {
		t.Fatalf("missing file: got %q", got)
	}
	body := append([]byte("FFCP"), bytes.Repeat([]byte{7}, 100)...)
	if err := os.WriteFile(filepath.Join(dir, "VDEE.bin"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dlsPackage("test", f); !bytes.Equal(got, body) {
		t.Fatalf("file: got %d bytes", len(got))
	}
	if err := os.WriteFile(filepath.Join(dir, "VDEE.bin"), make([]byte, pkgMaxLen+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dlsPackage("test", f); string(got) != "NONE" {
		t.Fatalf("oversize: got %d bytes", len(got))
	}
}
