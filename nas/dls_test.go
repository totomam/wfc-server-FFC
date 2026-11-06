package nas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDlsListContents(t *testing.T) {
	old := dlcDir
	dlcDir = t.TempDir()
	defer func() { dlcDir = old }()
	d := filepath.Join(dlcDir, "VDEE")
	os.MkdirAll(filepath.Join(d, "sub"), 0o755)
	os.WriteFile(filepath.Join(d, "1"), []byte("Rock one."), 0o644)
	os.WriteFile(filepath.Join(d, "3"), []byte("Rock three!"), 0o644)
	os.WriteFile(filepath.Join(d, ".hidden"), []byte("x"), 0o644)
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if got := string(dlsCount("t", f)); got != "2\x00" {
		t.Fatalf("count %q", got)
	}
	f["offset"], f["num"] = []byte("0"), []byte("2")
	l := string(dlsList("t", f))
	if l != "1\tQm9udXMgRGF0YSAx\t\t\t\t9\r\n3\tQm9udXMgRGF0YSAz\t\t\t\t11\r\n" {
		t.Fatalf("list %q", l)
	}
	f["offset"], f["num"] = []byte("1"), []byte("5")
	if l := string(dlsList("t", f)); !strings.HasPrefix(l, "3\t") || strings.Count(l, "\r\n") != 1 {
		t.Fatalf("list offset %q", l)
	}
	f["contents"] = []byte("3")
	if c := string(dlsContents("t", f)); c != "Rock three!" {
		t.Fatalf("contents %q", c)
	}
	for _, bad := range []string{"../VDEE/1", "sub", ".hidden", "2"} {
		f["contents"] = []byte(bad)
		if c := dlsContents("t", f); c != nil {
			t.Fatalf("served %q", bad)
		}
	}
}
