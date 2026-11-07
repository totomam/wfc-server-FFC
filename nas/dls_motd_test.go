package nas

import (
	"testing"
	"wwfc/ffcbonus"
)

func TestDlsMotd(t *testing.T) {
	old := ffcbonus.Dir
	ffcbonus.Dir = t.TempDir()
	defer func() { ffcbonus.Dir = old }()
	f := map[string][]byte{"rhgamecd": []byte("VDEE")}
	if b := dlsMotd("t", f); len(b) != 0 {
		t.Fatal("no motd:", b)
	}
	if err := ffcbonus.SetMotd("VDEE", "Hello"); err != nil {
		t.Fatal(err)
	}
	if b := dlsMotd("t", f); string(b) != "Hello" {
		t.Fatal(string(b))
	}
	if _, ok := dlsActions["motd"]; !ok {
		t.Fatal("action not registered")
	}
}
