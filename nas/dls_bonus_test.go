package nas

import (
	"strings"
	"testing"
	"wwfc/ffcbonus"
)

// A message written through ffcbonus (admin API) is what count/list/contents serve; the temp file never shows.
func TestBonusMessageServed(t *testing.T) {
	old := dlcDir
	dlcDir = t.TempDir()
	ffcbonus.Dir = dlcDir
	defer func() { dlcDir = old }()
	if err := ffcbonus.Set("VDEE", 12, "Tournament Saturday 18:00 UTC"); err != nil {
		t.Fatal(err)
	}
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if c := string(dlsCount("t", f)); c != "1\x00" {
		t.Fatalf("count %q", c)
	}
	if l := string(dlsList("t", f)); !strings.HasPrefix(l, "12\t") || !strings.HasSuffix(l, "\t29\r\n") {
		t.Fatalf("list %q", l)
	}
	f["contents"] = []byte("12")
	if b := string(dlsContents("t", f)); b != "Tournament Saturday 18:00 UTC" {
		t.Fatalf("contents %q", b)
	}
}
