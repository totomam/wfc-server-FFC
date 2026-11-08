package nas

import "testing"

type fakeLbNames struct {
	pids  []uint32
	names map[uint32]string
}

func (f *fakeLbNames) ProfilesOfUser(userID uint64, gamecd string) ([]uint32, error) {
	return f.pids, nil
}
func (f *fakeLbNames) LbNameOf(pids []uint32) (string, []uint32, error) {
	name, missing := "", []uint32{}
	for _, p := range pids {
		if n, ok := f.names[p]; !ok {
			missing = append(missing, p)
		} else if name == "" {
			name = n
		}
	}
	return name, missing, nil
}
func (f *fakeLbNames) SetLbName(pids []uint32, name string) error {
	for _, p := range pids {
		f.names[p] = name
	}
	return nil
}

func TestDlsLbName(t *testing.T) {
	old, oldBad := lbNameDB, lbNameIsBad
	defer func() { lbNameDB, lbNameIsBad = old, oldBad }()
	f := &fakeLbNames{pids: []uint32{11, 12}, names: map[uint32]string{}}
	lbNameDB = f
	lbNameIsBad = func(w string) (bool, error) { return w == "badword", nil }
	req := func(attr1 string, set bool) string {
		fl := map[string][]byte{"userid": []byte("4242"), "rhgamecd": []byte("VDEE")}
		if set {
			fl["attr1"] = []byte(attr1)
		}
		return string(dlsLbName("T", fl))
	}
	if got := req("", false); got != "0" {
		t.Fatalf("get before set = %q", got)
	}
	for _, bad := range []string{"   ", "badword", "my badword", "bad word", "x\x01y", "12345678901234567", "\xff"} {
		if got := req(bad, true); got != "bad" {
			t.Errorf("set %q = %q, want bad", bad, got)
		}
	}
	if got := req("  Rex   Fan ", true); got != "ok" || f.names[11] != "Rex Fan" || f.names[12] != "Rex Fan" {
		t.Fatalf("set = %q, names %v", got, f.names)
	}
	if got := req("", false); got != "1" {
		t.Fatalf("get after set = %q", got)
	}
	f.pids = append(f.pids, 13) // a new save on the same console
	if got := req("", false); got != "1" || f.names[13] != "Rex Fan" {
		t.Fatalf("new profile: %q %v", got, f.names)
	}
	if got := string(dlsLbName("T", map[string][]byte{"rhgamecd": []byte("VDEE")})); got != "err" {
		t.Fatalf("no userid = %q", got)
	}
	if _, ok := dlsActions["lbname"]; !ok {
		t.Fatal("lbname not registered")
	}
}
