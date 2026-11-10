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
	if err := os.MkdirAll(filepath.Join(d, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "1"), []byte("Rock one."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "3"), []byte("Rock three!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
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

// TestDlsListOverflow (P1-4.10): offset+num must not overflow into a
// negative slice end; negative values are rejected, num is clamped.
func TestDlsListOverflow(t *testing.T) {
	old := dlcDir
	dlcDir = t.TempDir()
	defer func() { dlcDir = old }()
	d := filepath.Join(dlcDir, "VDEE")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1", "2", "3"} {
		if err := os.WriteFile(filepath.Join(d, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dlsList panicked: %v", r)
		}
	}()
	cases := []struct {
		offset, num string
		lines       int
	}{
		{"0", "9223372036854775807", 3},
		{"1", "9223372036854775807", 2},
		{"3", "9223372036854775807", 0},
		{"9223372036854775807", "9223372036854775807", 0},
		{"-1", "2", 0},
		{"0", "-5", 0},
		{"-9223372036854775808", "9223372036854775807", 0},
		{"0", "2", 2},
	}
	for _, c := range cases {
		f := map[string][]byte{"rhgamecd": []byte("VDEE"), "offset": []byte(c.offset), "num": []byte(c.num)}
		got := string(dlsList("t", f))
		if n := strings.Count(got, "\r\n"); n != c.lines {
			t.Errorf("offset=%s num=%s: %d lines (%q), want %d", c.offset, c.num, n, got, c.lines)
		}
	}
}
