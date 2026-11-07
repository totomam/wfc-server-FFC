package database

import (
	"os"
	"testing"
	"time"
	"wwfc/common"
)

// Live (FFC_TEST_PG=1): a contradicting pair -> mismatch row with the field, listed by LoadFlaggedMatches; a lone
// report -> single row; FlaggedCounts counts both sides of the mismatch.
func TestFlaggedPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(db.ctx, `DELETE FROM match_results WHERE pid_a >= 900000100 AND pid_a < 900000200`); err != nil {
		t.Fatal(err)
	}
	team := []byte(`[{"slot":0,"creature":102,"rank_index":2}]`)
	r := MatchReport{PidA: 900000101, PidB: 900000102, WinnerPid: 900000101, NameA: "Liar", NameB: "Honest",
		TeamA: team, TeamB: team, ReporterIsA: true, Raw: []byte{1}}
	if res, err := db.StoreMatchReportDetail(r); err != nil || res.Status != "single" || res.Mismatch != "" {
		t.Fatal(res, err)
	}
	r.ReporterIsA, r.Raw, r.WinnerPid = false, []byte{2}, 900000102 // the other console says B won
	res, err := db.StoreMatchReportDetail(r)
	if err != nil || res.Status != "mismatch" || res.Mismatch != "winner" {
		t.Fatal(res, err)
	}
	lone := MatchReport{PidA: 900000103, PidB: 900000104, Draw: true, NameA: "X", NameB: "Y", TeamA: team, TeamB: team,
		ReporterIsA: true, Raw: []byte{3}}
	if _, err := db.StoreMatchReportDetail(lone); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-time.Hour)
	mm, err := db.LoadFlaggedMatches([]string{"mismatch"}, since, 100)
	if err != nil {
		t.Fatal(err)
	}
	var hit *FlaggedMatch
	for i := range mm {
		if mm[i].ID == res.ID {
			hit = &mm[i]
		}
		if mm[i].Status != "mismatch" {
			t.Fatal("non-mismatch row listed", mm[i])
		}
	}
	if hit == nil || hit.Mismatch != "winner" || !hit.ReportedA || !hit.ReportedB || hit.WinnerPid != 900000101 {
		t.Fatalf("mismatch row: %+v", hit)
	}
	both, err := db.LoadFlaggedMatches([]string{"mismatch", "single"}, since, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range both {
		if m.PidA == 900000103 && m.Status == "single" && m.ReportedA && !m.ReportedB {
			found = true
		}
	}
	if !found {
		t.Fatal("single row not listed")
	}
	if n := FlaggedCounts(mm); n[900000101] < 1 || n[900000102] < 1 {
		t.Fatal(n)
	}
}

func TestFlaggedCounts(t *testing.T) {
	n := FlaggedCounts([]FlaggedMatch{{PidA: 1, PidB: 2, Status: "mismatch"}, {PidA: 1, PidB: 3, Status: "mismatch"},
		{PidA: 4, PidB: 5, Status: "single"}})
	if n[1] != 2 || n[2] != 1 || n[3] != 1 || n[4] != 0 {
		t.Fatal(n)
	}
}
