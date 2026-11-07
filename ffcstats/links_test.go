package ffcstats

import "testing"

func TestApplyLinks(t *testing.T) {
	ms := []Match{
		{ID: 1, PidA: 10, PidB: 20, WinnerPid: 20},
		{ID: 2, PidA: 11, PidB: 20, WinnerPid: 11}, // 11 = alias of 10
		{ID: 3, PidA: 10, PidB: 11, WinnerPid: 10}, // self-match after linking -> dropped
		{ID: 4, PidA: 30, PidB: 11, Draw: true},
	}
	links := map[uint32]uint32{11: 10}
	got := ApplyLinks(ms, links)
	if len(got) != 3 || got[1].PidA != 10 || got[1].WinnerPid != 10 || got[2].PidB != 10 || got[2].WinnerPid != 0 {
		t.Fatalf("got %+v", got)
	}
	if ms[1].PidA != 11 {
		t.Fatal("input modified")
	}
	if ApplyLinks(ms, nil)[1].PidA != 11 {
		t.Fatal("nil links changed rows")
	}
	lb := Leaderboard(got, 0)
	for _, p := range lb {
		if p.Pid == 11 {
			t.Fatal("alias on the leaderboard")
		}
		if p.Pid == 10 && (p.Wins != 1 || p.Losses != 1 || p.Draws != 1) {
			t.Fatalf("canonical row %+v", p)
		}
	}
	if Resolve(links, 11) != 10 || Resolve(links, 12) != 12 {
		t.Fatal("resolve")
	}
}
