package database

import (
	"os"
	"testing"

	"wwfc/common"
)

// Live (FFC_TEST_PG=1), s119-online: reports that differ only in time_up (stale per-console scene+0x6b8 byte) are
// "confirmed"; the row keeps time_up only when both reports say so.
func TestTimeUpNotComparedPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(db.ctx, `DELETE FROM match_results WHERE pid_a >= 900000200 AND pid_a < 900000300`); err != nil {
		t.Fatal(err)
	}
	team := []byte(`[{"slot":0,"creature":102,"rank_index":2}]`)
	for i, tu := range [][2]bool{{false, true}, {true, false}, {true, true}} {
		r := MatchReport{PidA: uint32(900000201 + 2*i), PidB: uint32(900000202 + 2*i), WinnerPid: uint32(900000201 + 2*i),
			NameA: "A", NameB: "B", TeamA: team, TeamB: team, ReporterIsA: true, Raw: []byte{1}, TimeUp: tu[0]}
		if res, err := db.StoreMatchReportDetail(r); err != nil || res.Status != "single" {
			t.Fatal(res, err)
		}
		r.ReporterIsA, r.Raw, r.TimeUp = false, []byte{2}, tu[1]
		res, err := db.StoreMatchReportDetail(r)
		if err != nil || res.Status != "confirmed" || res.Mismatch != "" {
			t.Fatal(tu, res, err)
		}
		var got bool
		if err := db.pool.QueryRow(db.ctx, `SELECT time_up FROM match_results WHERE id = $1`, res.ID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != (tu[0] && tu[1]) {
			t.Fatalf("%v: time_up = %v", tu, got)
		}
	}
}
