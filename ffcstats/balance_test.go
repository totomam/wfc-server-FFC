package ffcstats

import (
	"testing"
	"time"
)

func TestBalanceImpact(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	a := []Pick{{Creature: 102}}
	b := []Pick{{Creature: 53}}
	ms := []Match{
		{PidA: 1, PidB: 2, WinnerPid: 1, MatchVer: 0x40000001, TeamA: a, TeamB: b, CreatedAt: t0},
		{PidA: 1, PidB: 2, WinnerPid: 1, MatchVer: 0x40000001, TeamA: a, TeamB: b, CreatedAt: t0.Add(time.Hour)},
		{PidA: 1, PidB: 2, WinnerPid: 2, MatchVer: 0x40000002, TeamA: a, TeamB: b, CreatedAt: t0.Add(48 * time.Hour)},
		{PidA: 1, PidB: 2, Draw: true, MatchVer: 90, TeamA: a, TeamB: b, CreatedAt: t0.Add(-time.Hour)},
	}
	rv := BalanceImpact(ms, 1)
	if len(rv) != 3 || rv[0].MatchVer != 90 || rv[0].Modded || rv[1].MatchVer != 0x40000001 || rv[2].MatchVer != 0x40000002 {
		t.Fatalf("%+v", rv)
	}
	v1 := rv[1]
	if v1.Matches != 2 || !v1.First.Equal(t0) || !v1.Last.Equal(t0.Add(time.Hour)) || len(v1.Vivosaurs) != 1 {
		t.Fatalf("%+v", v1)
	}
	if v1.Vivosaurs[0].Picks != 2 {
		t.Fatalf("%+v", v1.Vivosaurs)
	}
	// 102 wins under v1, loses under v2: the table change shows up
	find := func(r RulesVersion, c uint8) Vivosaur {
		for _, v := range BalanceImpact(ms, 0) {
			if v.MatchVer == r.MatchVer {
				for _, x := range v.Vivosaurs {
					if x.Creature == c {
						return x
					}
				}
			}
		}
		return Vivosaur{}
	}
	if find(rv[1], 102).WinRate != 1 || find(rv[2], 102).WinRate != 0 {
		t.Fatal("win rate per version")
	}
}
