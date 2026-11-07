package ffcstats

import (
	"strings"
	"testing"
	"time"
)

func TestResolveBracket(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 18, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }
	tr := Tournament{ID: "c", Start: t0, End: at(600), Mode: "casual", Players: []uint32{1, 2, 3, 4, 5}}
	m := func(id int64, a, b, w uint32, min int, q string) Match {
		return Match{ID: id, PidA: a, PidB: b, WinnerPid: w, CreatedAt: at(min), Queue: q, Status: "confirmed"}
	}
	ms := []Match{
		m(1, 1, 2, 1, -5, "casual"), // before start: ignored
		m(2, 2, 1, 2, 10, "ranked"), // other queue: ignored
		m(3, 2, 1, 2, 20, "casual"), // R1: 2 beats 1
		m(4, 3, 4, 4, 15, "casual"), // R1: 4 beats 3
		m(5, 3, 4, 3, 16, "casual"), // later rematch: does not change R1
		m(6, 4, 2, 4, 18, "casual"), // before 2's R1 win: does not count for R2
		m(7, 2, 4, 2, 30, "casual"), // R2: 2 beats 4
		m(8, 5, 2, 0, 40, "casual"), // draw-ish (winner 0): ignored
	}
	b := ResolveBracket(tr, ms)
	if len(b.Rounds) != 3 {
		t.Fatalf("rounds %d", len(b.Rounds))
	}
	if b.Rounds[0][0].Winner != 2 || b.Rounds[0][0].MatchID != 3 || b.Rounds[0][1].Winner != 4 || b.Rounds[0][1].MatchID != 4 {
		t.Fatalf("R1 %+v", b.Rounds[0])
	}
	if b.Rounds[0][2].Winner != 5 || b.Rounds[0][3].Winner != 0 { // 5 has a bye; (0,0) empty
		t.Fatalf("R1 byes %+v", b.Rounds[0][2:])
	}
	if b.Rounds[1][0].Winner != 2 || b.Rounds[1][0].MatchID != 7 || b.Rounds[1][1].Winner != 5 {
		t.Fatalf("R2 %+v", b.Rounds[1])
	}
	if b.Finished || b.Rounds[2][0].A != 2 || b.Rounds[2][0].B != 5 {
		t.Fatalf("final %+v finished=%v", b.Rounds[2], b.Finished)
	}
	b = ResolveBracket(tr, append(ms, m(9, 5, 2, 5, 50, "casual")))
	if !b.Finished || b.Champion != 5 {
		t.Fatalf("champion %d %v", b.Champion, b.Finished)
	}
	// undecided R1 match must not become a bye in R2
	b = ResolveBracket(tr, ms[:3])
	if b.Rounds[1][0].Winner != 0 {
		t.Fatalf("undecided treated as bye: %+v", b.Rounds[1][0])
	}
}

func TestParseTournaments(t *testing.T) {
	ok := `[{"id":"a","name":"A","start":"2026-11-14T18:00:00Z","players":[1,2]}]`
	if ts, err := ParseTournaments(strings.NewReader(ok)); err != nil || len(ts) != 1 {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`[{"id":"a","start":"2026-11-14T18:00:00Z","players":[1]}]`,
		`[{"id":"a","start":"2026-11-14T18:00:00Z","players":[1,1]}]`,
		`[{"id":"","start":"2026-11-14T18:00:00Z","players":[1,2]}]`,
		`[{"id":"a","players":[1,2]}]`,
		`[{"id":"a","start":"2026-11-14T18:00:00Z","mode":"x","players":[1,2]}]`,
	} {
		if _, err := ParseTournaments(strings.NewReader(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}
