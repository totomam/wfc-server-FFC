package database

import (
	"os"
	"testing"
	"time"

	"wwfc/common"
	"wwfc/ffcstats"
)

// Live Postgres check (skipped unless FFC_TEST_PG=1; local db wwfc/CHANGEME@127.0.0.1): rows written by
// StoreMatchReport come back through LoadMatches with teams decoded, and the window bounds hold.
func TestLoadMatchesPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(db.ctx, `DELETE FROM match_results WHERE pid_a >= 900000000 AND pid_a < 900000100`); err != nil {
		t.Fatal(err)
	}
	team := []byte(`[{"slot":0,"creature":102,"rank_index":2},{"slot":1,"creature":53,"rank_index":19}]`)
	r := MatchReport{PidA: 900000001, PidB: 900000002, WinnerPid: 900000002, NameA: "Dina", NameB: "Rex",
		TeamA: team, TeamB: team, ReporterIsA: true, Raw: []byte{1}}
	if _, st, err := db.StoreMatchReport(r); err != nil || st != "single" {
		t.Fatal(st, err)
	}
	r.ReporterIsA, r.Raw = false, []byte{2}
	if _, st, err := db.StoreMatchReport(r); err != nil || st != "confirmed" {
		t.Fatal(st, err)
	}
	ms, err := db.LoadMatches(time.Now().Add(-time.Hour), time.Time{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var got []ffcstats.Match
	for _, m := range ms {
		if m.PidA == 900000001 {
			got = append(got, m)
		}
	}
	if len(got) != 1 || got[0].Status != "confirmed" || got[0].WinnerPid != 900000002 || len(got[0].TeamB) != 2 ||
		got[0].TeamB[1].Creature != 53 || got[0].NameA != "Dina" {
		t.Fatalf("%+v", got)
	}
	lb := ffcstats.Leaderboard(ffcstats.Filter(got, nil, time.Time{}, time.Time{}, nil), 0)
	if len(lb) != 2 || lb[0].Pid != 900000002 || lb[0].Elo != 1016 {
		t.Fatalf("%+v", lb)
	}
	if old, _ := db.LoadMatches(time.Time{}, time.Now().Add(-time.Hour), 1000); len(old) != 0 {
		for _, m := range old {
			if m.PidA == 900000001 {
				t.Fatal("window upper bound")
			}
		}
	}
}
