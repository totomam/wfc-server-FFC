package ffcstats

import (
	"strings"
	"testing"
	"time"
)

func TestBanRules(t *testing.T) {
	at := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	rs, err := ParseBanRules(strings.NewReader(`[{"id":"r","creatures":[199]},{"id":"c","queue":"casual","modded":true,"creatures":[50],"start":"2026-11-01T00:00:00Z","end":"2026-11-10T00:00:00Z"}]`))
	if err != nil {
		t.Fatal(err)
	}
	team := func(c ...uint8) []Pick {
		var p []Pick
		for i, x := range c {
			p = append(p, Pick{Slot: i, Creature: x})
		}
		return p
	}
	ms := []Match{
		{ID: 1, PidA: 1, PidB: 2, TeamA: team(3, 199), TeamB: team(4), CreatedAt: at},                                              // ranked, banned A
		{ID: 2, PidA: 1, PidB: 2, TeamA: team(199), TeamB: team(4), Queue: "casual", CreatedAt: at},                                // casual: rule r not
		{ID: 3, PidA: 1, PidB: 2, TeamA: team(5), TeamB: team(50), Queue: "casual", Modded: true, CreatedAt: at},                   // rule c, B
		{ID: 4, PidA: 1, PidB: 2, TeamA: team(5), TeamB: team(50), Queue: "casual", Modded: false, CreatedAt: at},                  // vanilla: no
		{ID: 5, PidA: 1, PidB: 2, TeamA: team(50), TeamB: team(50), Queue: "casual", Modded: true, CreatedAt: at.AddDate(0, 0, 9)}, // after end
	}
	keep := DropBanned(ms, rs)
	if len(keep) != 3 || keep[0].ID != 2 || keep[1].ID != 4 || keep[2].ID != 5 {
		t.Fatalf("keep %+v", keep)
	}
	v := Violations(ms, rs)
	if len(v) != 2 || v[0].Rule != "r" || v[0].Pids[0] != 1 || v[1].Rule != "c" || v[1].Pids[0] != 2 || v[1].Found[0] != 50 {
		t.Fatalf("violations %+v", v)
	}
	if len(DropBanned(ms, nil)) != 5 {
		t.Fatal("no rules must keep all")
	}
	for _, bad := range []string{`[{"id":"x","creatures":[]}]`, `[{"id":"x","queue":"q","creatures":[1]}]`, `[{"creatures":[1]}]`,
		`[{"id":"x","creatures":[1],"start":"2026-11-02T00:00:00Z","end":"2026-11-01T00:00:00Z"}]`, `{`} {
		if _, err := ParseBanRules(strings.NewReader(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
