package nas

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDlsPackageEvent(t *testing.T) {
	dir, bdir := t.TempDir(), t.TempDir()
	oldP, oldB := packageDir, balanceDir
	packageDir, balanceDir = dir, bdir
	defer func() { packageDir, balanceDir = oldP, oldB }()
	w := func(d, n, s string) {
		if err := os.WriteFile(filepath.Join(d, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(dir, "VDEE.bin", "base")
	w(dir, "VDEE.xmas.bin", "xmas")
	w(bdir, "VDEE.xmas.bin", "xmastable")
	w(dir, eventsFile, "# event\tstart\tend\nnofile\t2026-12-01\t2027-01-01\nxmas\t2026-12-20\t2026-12-27\n"+
		"../evil\t2000-01-01\t2100-01-01\nbad\t2026-12-27\t2026-12-20\n")
	cases := []struct {
		now, pkg, table string
	}{
		{"2026-11-07T12:00:00Z", "VDEE.bin", "VDEE.bin"},
		{"2026-12-19T23:59:59Z", "VDEE.bin", "VDEE.bin"}, // "nofile" open but has no file -> next window / base
		{"2026-12-20T00:00:00Z", "VDEE.xmas.bin", "VDEE.xmas.bin"},
		{"2026-12-26T23:59:59Z", "VDEE.xmas.bin", "VDEE.xmas.bin"},
		{"2026-12-27T00:00:00Z", "VDEE.bin", "VDEE.bin"}, // end exclusive
	}
	for _, c := range cases {
		now, _ := time.Parse(time.RFC3339, c.now)
		if got := filepath.Base(activeEventFile(dir, "VDEE", now)); got != c.pkg {
			t.Errorf("%s package: got %s want %s", c.now, got, c.pkg)
		}
		if got := filepath.Base(activeEventFile(bdir, "VDEE", now)); got != c.table {
			t.Errorf("%s table: got %s want %s", c.now, got, c.table)
		}
	}
	if n := len(readPkgEvents(dir)); n != 2 {
		t.Errorf("parsed %d events, want 2 (path name and reversed window refused)", n)
	}
	// live action with an always-open window: the event file is served
	w(dir, eventsFile, "xmas\t2000-01-01\t2200-01-01\n")
	if got := dlsPackage("test", map[string][]byte{"rhgamecd": []byte("VDEE")}); !bytes.Equal(got, []byte("xmas")) {
		t.Errorf("dlsPackage event: got %q", got)
	}
	os.Remove(filepath.Join(dir, eventsFile))
	if got := dlsPackage("test", map[string][]byte{"rhgamecd": []byte("VDEE")}); !bytes.Equal(got, []byte("base")) {
		t.Errorf("dlsPackage: got %q", got)
	}
}
