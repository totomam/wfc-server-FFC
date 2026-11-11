package database

import (
	"os"
	"testing"
	"time"

	"wwfc/common"
	"wwfc/ffcstats"
)

// NET-35 (live Postgres, FFC_TEST_PG=1): a stored single report of a remembered pair becomes, once older than the
// forfeit timeout, a loss (W-L + Elo) for the profile that did not report; before the timeout it does not count.
func TestSingleReportForfeit(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	const a, b = 920000001, 920000002
	clean := func() {
		if _, err := db.pool.Exec(db.ctx, `DELETE FROM match_results WHERE pid_a = $1 AND pid_b = $2`, a, b); err != nil {
			t.Fatal(err)
		}
	}
	clean()
	defer clean()
	team := []byte(`[{"slot":0,"creature":102,"rank_index":2}]`)
	res, err := db.StoreMatchReportDetail(MatchReport{PidA: a, PidB: b, WinnerPid: b, NameA: "A", NameB: "B", TeamA: team, TeamB: team,
		ReporterIsA: false, Raw: []byte{1}, PairSeen: true}) // b reported a win; a never reports (quit = loss for a)
	if err != nil || res.Status != "single" {
		t.Fatal(res, err)
	}
	stat := func() (ffcstats.Player, ffcstats.Player) {
		ms, err := db.LoadMatches(time.Time{}, time.Time{}, 100000)
		if err != nil {
			t.Fatal(err)
		}
		var mine []ffcstats.Match
		for _, m := range ms {
			if m.PidA == a && m.PidB == b {
				mine = append(mine, m)
			}
		}
		pa, pb := ffcstats.Player{Elo: ffcstats.EloStart}, ffcstats.Player{Elo: ffcstats.EloStart}
		for _, p := range ffcstats.Leaderboard(ffcstats.Filter(mine, nil, time.Time{}, time.Time{}, nil), 0) {
			switch p.Pid {
			case a:
				pa = p
			case b:
				pb = p
			}
		}
		return pa, pb
	}
	if pa, pb := stat(); pa.Losses != 0 || pa.Elo != ffcstats.EloStart || pb.Wins != 0 {
		t.Fatalf("before timeout: %+v %+v", pa, pb)
	}
	if _, err := db.pool.Exec(db.ctx, `UPDATE match_results SET created_at = created_at - interval '16 minutes' WHERE id = $1`, res.ID); err != nil {
		t.Fatal(err)
	}
	if pa, pb := stat(); pa.Losses != 1 || pa.Elo >= ffcstats.EloStart || pb.Wins != 1 || pb.Elo <= ffcstats.EloStart {
		t.Fatalf("after timeout: non-reporter %+v reporter %+v", pa, pb)
	}
}
