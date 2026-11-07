package ffclimit

import (
	"testing"
	"time"
)

func TestBucket(t *testing.T) {
	Reset()
	now := time.Unix(100, 0)
	Now = func() time.Time { return now }
	r := Rule{"t", 3, 1}
	for i := 0; i < 3; i++ {
		if !Allow(r, "1.2.3.4") {
			t.Fatal("burst", i)
		}
	}
	if Allow(r, "1.2.3.4") {
		t.Fatal("over burst allowed")
	}
	if !Allow(r, "5.6.7.8") {
		t.Fatal("other ip must have its own bucket")
	}
	now = now.Add(1500 * time.Millisecond)
	if !Allow(r, "1.2.3.4") || Allow(r, "1.2.3.4") {
		t.Fatal("refill 1/s")
	}
	if Counts()["t"] != 2 {
		t.Fatal(Counts())
	}
	Disabled.Store(true)
	if !Allow(r, "1.2.3.4") {
		t.Fatal("disabled")
	}
	Disabled.Store(false)
}

func TestIPOf(t *testing.T) {
	for in, want := range map[string]string{"1.2.3.4:5": "1.2.3.4", "[::1]:80": "::1", "1.2.3.4": "1.2.3.4"} {
		if got := IPOf(in); got != want {
			t.Fatal(in, got)
		}
	}
}

// A steady QR2 stream of two consoles behind one IP (each ~1 heartbeat/keepalive per s plus match-command bursts of
// ~40 packets) must never be refused.
func TestPairNeverLimited(t *testing.T) {
	Reset()
	now := time.Unix(1000, 0)
	Now = func() time.Time { return now }
	for sec := 0; sec < 600; sec++ {
		n := 4
		if sec%30 == 0 {
			n = 80
		}
		for i := 0; i < n; i++ {
			if !Allow(QR2Packet, "10.0.0.1") || !Allow(NATNEG, "10.0.0.1") {
				t.Fatal("pair limited at", sec)
			}
		}
		if sec%3 == 0 && !Allow(SBPacket, "10.0.0.1") {
			t.Fatal("sb", sec)
		}
		now = now.Add(time.Second)
	}
}
