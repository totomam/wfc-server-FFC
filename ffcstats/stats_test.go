package ffcstats

import (
	"math"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func mk(id int64, a, b, w uint32, mins int, teamA, teamB []uint8) Match {
	m := Match{ID: id, PidA: a, PidB: b, WinnerPid: w, Draw: w == 0, Status: "confirmed", NameA: "P", NameB: "Q",
		CreatedAt: t0.Add(time.Duration(mins) * time.Minute)}
	for i, c := range teamA {
		m.TeamA = append(m.TeamA, Pick{Slot: i, Creature: c})
	}
	for i, c := range teamB {
		m.TeamB = append(m.TeamB, Pick{Slot: i, Creature: c})
	}
	return m
}

func TestEloFirstMatch(t *testing.T) {
	lb := Leaderboard([]Match{mk(1, 10, 20, 10, 0, nil, nil)}, 0)
	if len(lb) != 2 || lb[0].Pid != 10 || lb[0].Elo != 1016 || lb[1].Elo != 984 || lb[0].Rank != 1 {
		t.Fatalf("got %+v", lb)
	}
	if lb[0].Wins != 1 || lb[1].Losses != 1 || lb[0].WinRate != 1 || lb[0].Peak != 1016 || lb[1].Peak != 1000 {
		t.Fatalf("record %+v", lb)
	}
}

func TestEloDrawAndOrder(t *testing.T) {
	// out-of-order input: replay is chronological (A wins first, then the draw)
	ms := []Match{mk(2, 10, 20, 0, 5, nil, nil), mk(1, 10, 20, 10, 0, nil, nil)}
	lb := Leaderboard(ms, 0)
	// after win: 1016/984; draw: expected A = 1/(1+10^(-32/400)) = 0.5460, d = 32*(0.5-0.546) = -1.47
	if math.Abs(lb[0].Elo-1014.5) > 0.11 || math.Abs(lb[1].Elo-985.5) > 0.11 || lb[0].Draws != 1 {
		t.Fatalf("got %+v", lb)
	}
	if lb[0].Played != 2 || lb[0].WinRate != 0.5 {
		t.Fatalf("played %+v", lb[0])
	}
}

func TestEloZeroSumAndMinPlayed(t *testing.T) {
	ms := []Match{mk(1, 1, 2, 1, 0, nil, nil), mk(2, 2, 3, 3, 1, nil, nil), mk(3, 1, 3, 3, 2, nil, nil),
		mk(4, 1, 2, 2, 3, nil, nil), mk(5, 9, 9, 9, 4, nil, nil), mk(6, 1, 2, 77, 5, nil, nil)}
	lb := Leaderboard(ms, 0)
	sum := 0.0
	for _, p := range lb {
		sum += p.Elo
	}
	if len(lb) != 3 || math.Abs(sum-3000) > 0.2 {
		t.Fatalf("self-match / bad winner must be skipped, Elo zero-sum: %+v", lb)
	}
	if lb[0].Pid != 3 || lb[0].Wins != 2 {
		t.Fatalf("leader %+v", lb[0])
	}
	if got := Leaderboard(ms, 3); len(got) != 2 || got[0].Pid != 2 || got[1].Rank != 2 {
		t.Fatalf("minPlayed %+v", got)
	}
}

func TestVivosaurs(t *testing.T) {
	ms := []Match{mk(1, 1, 2, 1, 0, []uint8{5, 7, 5}, []uint8{7, 9}), mk(2, 1, 2, 0, 1, []uint8{5}, []uint8{9})}
	rows, teams := Vivosaurs(ms)
	if teams != 4 || len(rows) != 3 {
		t.Fatalf("teams %d rows %+v", teams, rows)
	}
	// 5: picked by A twice (dup in one team counts once): 1 win, 1 draw
	if r := rows[0]; r.Creature != 5 || r.Picks != 2 || r.Wins != 1 || r.Draws != 1 || r.PickRate != 0.5 || r.WinRate != 0.5 {
		t.Fatalf("row5 %+v", r)
	}
	// 7 and 9 tie on picks (2) -> creature order; 7: win (A) + loss (B)
	if r := rows[1]; r.Creature != 7 || r.Wins != 1 || r.Losses != 1 {
		t.Fatalf("row7 %+v", r)
	}
	if r := rows[2]; r.Creature != 9 || r.Losses != 1 || r.Draws != 1 || r.Wins != 0 {
		t.Fatalf("row9 %+v", r)
	}
}

func TestH2HAndFilter(t *testing.T) {
	ms := []Match{mk(1, 1, 2, 1, 0, nil, nil), mk(2, 1, 2, 2, 1, nil, nil), mk(3, 1, 2, 2, 2, nil, nil),
		mk(4, 1, 2, 0, 3, nil, nil), mk(5, 1, 3, 1, 4, nil, nil)}
	h := H2H(ms, 2, 1)
	if h.WinsA != 2 || h.WinsB != 1 || h.Draws != 1 || len(h.Matches) != 4 || h.Matches[0].ID != 4 {
		t.Fatalf("h2h %+v", h)
	}
	ms[1].Status = "mismatch"
	ms[2].Modded = true
	yes := true
	if got := Filter(ms, nil, time.Time{}, time.Time{}, nil); len(got) != 4 {
		t.Fatalf("confirmed only: %d", len(got))
	}
	if got := Filter(ms, &yes, time.Time{}, time.Time{}, nil); len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("modded: %+v", got)
	}
	if got := Filter(ms, nil, t0.Add(time.Minute), t0.Add(3*time.Minute), []string{"confirmed", "mismatch"}); len(got) != 2 {
		t.Fatalf("window [1,3) min: %d", len(got))
	}
}

func TestParseTeam(t *testing.T) {
	p := ParseTeam([]byte(`[{"slot":0,"creature":102,"rank_index":2,"base_bp":5}]`))
	if len(p) != 1 || p[0].Creature != 102 || p[0].Rank != 2 {
		t.Fatalf("%+v", p)
	}
	if ParseTeam([]byte(`x`)) != nil {
		t.Fatal("bad json")
	}
}

func TestSeasons(t *testing.T) {
	ss, err := ParseSeasons(strings.NewReader("# id\tname\tstart\ns1\tLaunch\t2026-11-01\n\ns2\tWinter\t2026-12-01\n"))
	if err != nil || len(ss) != 2 || !ss[0].End.Equal(ss[1].Start) || !ss[1].End.IsZero() {
		t.Fatalf("%v %+v", err, ss)
	}
	if s, ok := FindSeason(ss, "current"); !ok || s.ID != "s2" {
		t.Fatal("current")
	}
	if _, ok := FindSeason(ss, "nope"); ok {
		t.Fatal("unknown")
	}
	for _, bad := range []string{"a\tb\n", "a\tb\t2026-13-01\n", "a\tA\t2026-12-01\nb\tB\t2026-11-01\n", "a\tA\t2026-11-01\na\tB\t2026-12-01\n"} {
		if _, err := ParseSeasons(strings.NewReader(bad)); err == nil {
			t.Fatalf("want error for %q", bad)
		}
	}
}
