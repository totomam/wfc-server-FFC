package nas

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDlsModes(t *testing.T) {
	dir := t.TempDir()
	oldP := packageDir
	packageDir = dir
	defer func() { packageDir = oldP }()
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if got := string(dlsModes("t", f)); got != "" {
		t.Fatalf("no modes.tsv: %q", got)
	}
	w := func(d, n, s string) {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(dir, modesFile, "# modes\ndefault\tHack rules\ndraft\tNo legendaries\n../x\tBad\nlong\t"+
		"aaaaaaaaaaaaaaaaaaaaa\nnodisp\nhard\tHard mode\nfifth\tToo many\nsixth\tX\n")
	want := "default\tHack rules\ndraft\tNo legendaries\nhard\tHard mode\nfifth\tToo many\n"
	if got := string(dlsModes("t", f)); got != want {
		t.Fatalf("modes: %q want %q", got, want)
	}
	w(dir, "VDEE.bin", "base")
	w(dir, "VDEE.draft.bin", "draft")
	now := time.Now()
	for mode, wantP := range map[string]string{"": "VDEE.bin", "default": "VDEE.bin", "draft": "VDEE.draft.bin",
		"hard": "VDEE.bin", "nope": "VDEE.bin", "../x": "VDEE.bin"} {
		g := map[string][]byte{"rhgamecd": []byte("VDEE"), "attr1": []byte(mode)}
		if got := filepath.Base(modeFile(dir, "VDEE", g, now)); got != wantP {
			t.Errorf("mode %q: %s want %s", mode, got, wantP)
		}
	}
}
