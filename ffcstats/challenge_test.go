package ffcstats

import (
	"strings"
	"testing"
	"time"
)

func TestChallenges(t *testing.T) {
	cs, err := ParseChallenges(strings.NewReader(`[
	 {"id":"w1","title":"Win 2 with Aeros","start":"2026-11-01T00:00:00Z","end":"2026-11-08T00:00:00Z","kind":"wins_with","creature":102,"target":2},
	 {"id":"g1","title":"Play 3","start":"2026-11-01T00:00:00Z","end":"2026-11-08T00:00:00Z","kind":"games","target":3,"queue":"all"}]`))
	if err != nil || len(cs) != 2 {
		t.Fatal(err)
	}
	at := func(h int) time.Time { return time.Date(2026, 11, 2, h, 0, 0, 0, time.UTC) }
	aeros := []Pick{{Creature: 102}}
	other := []Pick{{Creature: 5}}
	ms := []Match{
		{PidA: 1, PidB: 2, WinnerPid: 1, TeamA: aeros, TeamB: aeros, Status: "confirmed", CreatedAt: at(1)},
		{PidA: 1, PidB: 2, WinnerPid: 2, TeamA: aeros, TeamB: aeros, Status: "confirmed", CreatedAt: at(2)},
		{PidA: 1, PidB: 3, WinnerPid: 1, TeamA: aeros, TeamB: other, Status: "confirmed", CreatedAt: at(3), Queue: "casual"},
		{PidA: 2, PidB: 3, WinnerPid: 2, TeamA: other, TeamB: other, Status: "confirmed", CreatedAt: at(4)},
		{PidA: 1, PidB: 2, WinnerPid: 1, TeamA: aeros, TeamB: aeros, Status: "mismatch", CreatedAt: at(5)},
		{PidA: 1, PidB: 2, WinnerPid: 1, TeamA: aeros, TeamB: aeros, Status: "confirmed", CreatedAt: at(6)},
		{PidA: 1, PidB: 2, Draw: true, TeamA: aeros, TeamB: aeros, Status: "confirmed", CreatedAt: at(7)},
	}
	now := at(12)
	w := EvalChallenge(cs[0], ms, now)
	// ranked only: pid 1 wins with Aeros at 1h and 6h (3h is casual) -> done at 6h; pid 2 1 win (2h; 4h without Aeros).
	if !w.Active || w.Completed != 1 || w.Players[0].Pid != 1 || !w.Players[0].At.Equal(at(6)) || w.Players[1].Count != 1 {
		t.Fatalf("%+v", w)
	}
	g := EvalChallenge(cs[1], ms, now)
	// all queues, confirmed only: pid1 games 1,2,3 -> done at 3h; pid2 1,2,4 -> done at 4h; pid3 3,4 -> 2.
	if g.Completed != 2 || g.Players[0].Pid != 1 || g.Players[1].Pid != 2 || g.Players[2].Count != 2 {
		t.Fatalf("%+v", g)
	}
	if _, err := ParseChallenges(strings.NewReader(`[{"id":"x","start":"2026-11-01T00:00:00Z","end":"2026-11-02T00:00:00Z","kind":"wins_with","target":1}]`)); err == nil {
		t.Fatal("wins_with without creature accepted")
	}
}
