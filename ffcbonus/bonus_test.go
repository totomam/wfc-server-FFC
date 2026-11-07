package ffcbonus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBonusMessages(t *testing.T) {
	Dir = t.TempDir()
	if ms, err := List("VDEE"); err != nil || len(ms) != 0 {
		t.Fatal(ms, err)
	}
	if err := Set("VDEE", 9, "Season 2 starts Monday!\nGood luck."); err != nil {
		t.Fatal(err)
	}
	if err := Set("VDEE", 8, "first"); err != nil {
		t.Fatal(err)
	}
	if err := Set("VDEE", 8, "replaced"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(Dir, "VDEE", "notes.txt"), []byte("x"), 0o644) // not a message: not listed
	_ = os.WriteFile(filepath.Join(Dir, "VDEE", "09"), []byte("x"), 0o644)        // non-canonical id: not listed
	ms, err := List("VDEE")
	if err != nil || len(ms) != 2 || ms[0].ID != 8 || ms[0].Text != "replaced" || ms[1].ID != 9 {
		t.Fatal(ms, err)
	}
	for _, c := range []struct {
		id   int
		text string
		want error
	}{{0, "x", ErrID}, {97, "x", ErrID}, {5, "", ErrText}, {5, strings.Repeat("a", MaxText+1), ErrText},
		{5, "tab\there", ErrText}, {5, "caf\xc3\xa9", ErrText}} {
		if err := Set("VDEE", c.id, c.text); err != c.want {
			t.Fatal(c.id, err)
		}
	}
	if err := Set("../x", 1, "x"); err != ErrCode {
		t.Fatal(err)
	}
	if err := Set("VDEE", 10, strings.Repeat("a", MaxText)); err != nil {
		t.Fatal("max length:", err)
	}
	if err := Delete("VDEE", 8); err != nil {
		t.Fatal(err)
	}
	if err := Delete("VDEE", 8); err != nil {
		t.Fatal("second delete:", err)
	}
	if ms, _ := List("VDEE"); len(ms) != 2 || ms[0].ID != 9 {
		t.Fatal(ms)
	}
	ents, _ := os.ReadDir(filepath.Join(Dir, "VDEE"))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".msg-") {
			t.Fatal("temp file left:", e.Name())
		}
	}
}
