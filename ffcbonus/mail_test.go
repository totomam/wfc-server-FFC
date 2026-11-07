package ffcbonus

import (
	"strings"
	"testing"
)

func TestMail(t *testing.T) {
	Dir = t.TempDir()
	if ms, err := MailList("VDEE", 7); err != nil || len(ms) != 0 || MailItems("VDEE", 7) != nil {
		t.Fatal("empty inbox", ms, err)
	}
	for i := 1; i <= MailSlots; i++ {
		if s, err := MailSet("VDEE", 7, 0, "hello"); err != nil || s != i {
			t.Fatal(i, s, err)
		}
	}
	if _, err := MailSet("VDEE", 7, 0, "x"); err != ErrFull {
		t.Fatal("full:", err)
	}
	if _, err := MailSet("VDEE", 7, 17, "x"); err != ErrSlot {
		t.Fatal("slot 17:", err)
	}
	if _, err := MailSet("VDEE", 7, 1, strings.Repeat("a", MaxText)); err != ErrText {
		t.Fatal("too long:", err)
	}
	if err := MailDelete("VDEE", 7, 3); err != nil {
		t.Fatal(err)
	}
	if s, err := MailSet("VDEE", 7, 0, "Hi BOB\nReward: 5 wins"); err != nil || s != 3 {
		t.Fatal("refill slot 3:", s, err)
	}
	it := MailItems("VDEE", 7)
	if len(it) != MailSlots || it[0].Name != "165" || it[15].Name != "180" || string(it[2].Body) != "MAIL 3/16\nHi BOB\nReward: 5 wins" {
		t.Fatalf("items %q %q %q", it[0].Name, it[15].Name, it[2].Body)
	}
	if MailItems("VDEE", 8) != nil {
		t.Fatal("other player sees mail")
	}
	if !IsMailName("165") || !IsMailName("180") || IsMailName("181") || IsMailName("164") || IsMailName("0165") {
		t.Fatal("IsMailName")
	}
	if err := MailClear("VDEE", 7); err != nil || MailItems("VDEE", 7) != nil {
		t.Fatal("clear", err)
	}
	// the game keeps id % 100: every mail name lands in the 65..99 always-shown news range
	for s := 1; s <= MailSlots; s++ {
		n := 0
		for _, c := range MailName(s) {
			n = n*10 + int(c-'0')
		}
		if n%100 < 65 || n%100 > 99 {
			t.Fatal("slot", s, "->", n%100)
		}
	}
}
