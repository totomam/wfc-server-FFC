package ffcstats

import (
	"testing"
	"time"
)

func playerOf(lb []Player, pid uint32) Player {
	for _, p := range lb {
		if p.Pid == pid {
			return p
		}
	}
	return Player{Pid: pid, Elo: EloStart}
}

// NET-35 (#357, decision a: a quit counts as a loss): one report and no second one -> after the timeout the side that
// did not report loses (Elo + W-L); before the timeout nothing changes. Only remembered pairs (NET-10) qualify, and two
// late single rows from opposite reporters cancel out.
func TestForfeitAfterTimeout(t *testing.T) {
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	after := 15 * time.Minute
	// pid 10 reported a win, pid 20 never reported.
	single := Match{ID: 1, PidA: 10, PidB: 20, WinnerPid: 10, Status: "single", Reporter: 10, PairSeen: true, CreatedAt: t0}
	board := func(ms []Match, now time.Time) []Player {
		return Leaderboard(Filter(ApplyForfeits(ms, after, now), nil, time.Time{}, time.Time{}, nil), 0)
	}

	lb := board([]Match{single}, t0.Add(after-time.Second))
	if p := playerOf(lb, 20); p.Losses != 0 || p.Elo != EloStart || len(lb) != 0 {
		t.Fatalf("before timeout: %+v", lb)
	}
	lb = board([]Match{single}, t0.Add(after))
	loser, winner := playerOf(lb, 20), playerOf(lb, 10)
	if loser.Losses != 1 || loser.Wins != 0 || loser.Elo != EloStart-EloK/2 || winner.Wins != 1 || winner.Elo != EloStart+EloK/2 {
		t.Fatalf("after timeout: loser %+v winner %+v", loser, winner)
	}
	if single.Status != "single" || single.WinnerPid != 10 {
		t.Fatal("input row modified")
	}

	// the lone report admits its own loss: kept (20 wins), never turned into a win for the reporter
	admitted := single
	admitted.WinnerPid = 20
	lb = board([]Match{admitted}, t0.Add(after))
	if w, l := playerOf(lb, 20), playerOf(lb, 10); w.Wins != 1 || w.Losses != 0 || l.Losses != 1 || l.Wins != 0 {
		t.Fatalf("admitted loss: 20 %+v 10 %+v", w, l)
	}

	unpaired := single
	unpaired.PairSeen = false // stored before the NET-10 pair check: never a free win
	if lb := board([]Match{unpaired}, t0.Add(time.Hour)); len(lb) != 0 {
		t.Fatalf("unpaired row counted: %+v", lb)
	}
	other := Match{ID: 2, PidA: 10, PidB: 20, WinnerPid: 20, Status: "single", Reporter: 20, PairSeen: true, CreatedAt: t0.Add(11 * time.Minute)}
	if lb := board([]Match{single, other}, t0.Add(time.Hour)); len(lb) != 0 {
		t.Fatalf("contradicting single rows counted: %+v", lb)
	}
	if n := len(Filter(ApplyForfeits([]Match{single}, 0, t0.Add(time.Hour)), nil, time.Time{}, time.Time{}, nil)); n != 0 {
		t.Fatal("after <= 0 must disable forfeits")
	}
}
