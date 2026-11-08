package nas

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDlsBoot(t *testing.T) {
	dir := t.TempDir()
	old := bootDir
	bootDir = dir
	defer func() { bootDir = old }()
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if got := dlsBoot("test", f); string(got) != "NONE" {
		t.Fatalf("missing: %q", got)
	}
	man := append([]byte("FFCM"), bytes.Repeat([]byte{1}, 200)...)
	if err := os.WriteFile(filepath.Join(dir, "VDEE.bin"), man, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dlsBoot("test", f); !bytes.Equal(got, man) {
		t.Fatalf("manifest: %d bytes", len(got))
	}
	if err := os.WriteFile(filepath.Join(dir, "VDEE.bin"), []byte("FFCPxxxx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := dlsBoot("test", f); string(got) != "NONE" {
		t.Fatalf("not a manifest: %q", got)
	}
	if err := os.MkdirAll(filepath.Join(dir, "parts"), 0o755); err != nil {
		t.Fatal(err)
	}
	part := make([]byte, 40000)
	for i := range part {
		part[i] = byte(i * 7)
	}
	if err := os.WriteFile(filepath.Join(dir, "parts", "0123456789abcdef.bin"), part, 0o644); err != nil {
		t.Fatal(err)
	}
	q := func(c, h string) []byte {
		return dlsBootPart("test", map[string][]byte{"rhgamecd": []byte("VDEE"), "attr1": []byte("0"), "attr2": []byte(c), "attr3": []byte(h)})
	}
	if got := q("0", "0123456789abcdef"); !bytes.Equal(got, part[:16384]) {
		t.Fatal("chunk 0")
	}
	if got := q("2", "0123456789abcdef"); !bytes.Equal(got, part[32768:]) {
		t.Fatal("last chunk")
	}
	for _, c := range [][2]string{{"3", "0123456789abcdef"}, {"-1", "0123456789abcdef"}, {"0", "../../etc/passwd"}, {"0", "0123456789ABCDEF"}, {"x", "0123456789abcdef"}} {
		if got := q(c[0], c[1]); string(got) != "NONE" {
			t.Fatalf("%v: %q", c, got)
		}
	}
}
