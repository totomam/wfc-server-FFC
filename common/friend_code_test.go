package common

import "testing"

func TestFriendCodeDSUsesMD5(t *testing.T) {
	// Codes the game (Fossil Fighters: Champions, VDEE) shows and accepts in "Register a Friend".
	cases := []struct {
		pid  uint32
		game string
		want string
	}{
		{1000000000, "VDEE", "2243-3829-9392"},
		{1000000001, "VDEE", "0439-4967-2961"},
	}
	for _, c := range cases {
		if got := CalcFriendCodeString(c.pid, c.game); got != c.want {
			t.Errorf("CalcFriendCodeString(%d, %q) = %s, want %s", c.pid, c.game, got, c.want)
		}
	}
}

func TestFriendCodeWiiSpecialCasesUnchanged(t *testing.T) {
	if typ, rev := getCRCType("RSBE"); typ != fcCRC8 || rev {
		t.Errorf("RSBE: got %d %v", typ, rev)
	}
	if typ, rev := getCRCType("HDME"); typ != fcCRC8 || !rev {
		t.Errorf("HDME: got %d %v", typ, rev)
	}
	if typ, _ := getCRCType("RMCE"); typ != fcMD5 {
		t.Errorf("RMCE: got %d", typ)
	}
}
