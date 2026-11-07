package ffcbonus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMotd(t *testing.T) {
	Dir = t.TempDir()
	if s, err := Motd("VDEE"); err != nil || s != "" {
		t.Fatal("empty:", s, err)
	}
	if err := SetMotd("VDEE", "Welcome!\nSeason 2 starts Monday."); err != nil {
		t.Fatal(err)
	}
	if s, _ := Motd("VDEE"); s != "Welcome!\nSeason 2 starts Monday." {
		t.Fatal(s)
	}
	if err := SetMotd("VDEE", strings.Repeat("x", MotdMax+1)); err != ErrMotd {
		t.Fatal("too long:", err)
	}
	if err := SetMotd("VDEE", "bad\x01"); err != ErrMotd {
		t.Fatal("control char:", err)
	}
	if err := SetMotd("../x", "hi"); err != ErrCode {
		t.Fatal("code:", err)
	}
	if fs, _ := List("VDEE"); len(fs) != 0 {
		t.Fatal("motd listed as Bonus Data:", fs)
	}
	os.WriteFile(filepath.Join(Dir, "VDEE.motd"), []byte(strings.Repeat("y", MotdMax+5)), 0o644)
	if s, _ := Motd("VDEE"); s != "" {
		t.Fatal("oversized file served")
	}
	if err := DeleteMotd("VDEE"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteMotd("VDEE"); err != nil {
		t.Fatal("second delete:", err)
	}
	if s, _ := Motd("VDEE"); s != "" {
		t.Fatal("after delete:", s)
	}
}
