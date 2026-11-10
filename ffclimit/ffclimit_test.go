package ffclimit

import (
	"fmt"
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

// NET-09: once the map is full, a new IP must not cost a full-map scan. 100,000 distinct sources (twice maxKeys)
// must average under 10 us per Allow call.
func TestAllowManyDistinctIPsFast(t *testing.T) {
	Reset()
	defer Reset()
	now := time.Unix(5000, 0)
	Now = func() time.Time { return now }
	defer func() { Now = time.Now }()
	const n = 100000
	ips := make([]string, n)
	for i := range ips {
		ips[i] = fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255)
	}
	start := time.Now()
	for i, ip := range ips {
		if i%1000 == 0 {
			now = now.Add(time.Millisecond) // all buckets stay younger than the idle expiry
		}
		Allow(QR2Packet, ip)
	}
	if per := time.Since(start) / n; per > 10*time.Microsecond {
		t.Fatalf("%v per Allow with %d distinct IPs (want < 10us)", per, n)
	}
	if k := Keys(); k > maxKeys {
		t.Fatalf("%d buckets kept, maxKeys %d", k, maxKeys)
	}
}

func BenchmarkAllowDistinctIPs(b *testing.B) {
	Reset()
	defer Reset()
	Now = time.Now
	ips := make([]string, 100000)
	for i := range ips {
		ips[i] = fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Allow(QR2Packet, ips[i%len(ips)])
	}
}

// Idle buckets are expired so the map does not fill up with old sources.
func TestIdleBucketsExpire(t *testing.T) {
	Reset()
	defer Reset()
	now := time.Unix(9000, 0)
	Now = func() time.Time { return now }
	defer func() { Now = time.Now }()
	for i := 0; i < 1000; i++ {
		Allow(NATNEG, fmt.Sprintf("10.0.%d.%d", i>>8, i&255))
	}
	now = now.Add(10 * time.Minute)
	for i := 0; i < 1000; i++ {
		Allow(NATNEG, fmt.Sprintf("10.1.%d.%d", i>>8, i&255))
	}
	if k := Keys(); k > 1000 {
		t.Fatalf("%d buckets kept after the first 1000 went idle", k)
	}
}
