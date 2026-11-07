package api

import (
	"os"
	"testing"
	"wwfc/ffcstats"
)

func TestFFCBadgeBuild(t *testing.T) {
	rows := []ffcstats.Player{{Pid: 1}, {Pid: 2}, {Pid: 3}, {Pid: 4}}
	m := ffcBadgeBuild(rows, map[uint32]string{2: "CHAMP", 9: "MOD"})
	if m[1] != "#1" || m[2] != "CHAMP" || m[3] != "#3" || m[4] != "" || m[9] != "MOD" {
		t.Fatalf("%v", m)
	}
	f := t.TempDir() + "/badges.json"
	old := ffcBadgesFile
	defer func() { ffcBadgesFile = old }()
	ffcBadgesFile = f
	_ = os.WriteFile(f, []byte(`{"10":"CHAMP","x":"A","11":"TOOLONG","12":"a b","13":"OK"}`), 0o644)
	if a := ffcBadgeFile(); len(a) != 2 || a[10] != "CHAMP" || a[13] != "OK" {
		t.Fatalf("file %v", a)
	}
}
