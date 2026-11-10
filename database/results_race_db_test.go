package database

import (
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"

	"wwfc/common"
)

// NET-11 (live Postgres, FFC_TEST_PG=1): 40 matches whose two consoles report at the same instant must each end as
// ONE confirmed row (audit: 33 of 40 ended as two "single" rows), and a repeated report from one side adds no row.
func TestStoreMatchReportConcurrentPairs(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	// pool_max_conns: let all 80 reports hold a connection at once, as 80 NAS requests would.
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc?pool_max_conns=100"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	const base, pairs = 910000000, 40
	clean := func() {
		if _, err := db.pool.Exec(db.ctx, `DELETE FROM match_results WHERE pid_a >= $1 AND pid_a < $2`, base, base+1000); err != nil {
			t.Fatal(err)
		}
	}
	clean()
	defer clean()
	team := []byte(`[{"slot":0,"creature":102,"rank_index":2}]`)
	report := func(i int, isA bool) MatchReport {
		return MatchReport{PidA: uint32(base + 2*i), PidB: uint32(base + 2*i + 1), WinnerPid: uint32(base + 2*i), NameA: "A", NameB: "B",
			TeamA: team, TeamB: team, ReporterIsA: isA, Raw: []byte{byte(i)}}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2*pairs)
	for i := 0; i < pairs; i++ {
		for _, isA := range []bool{true, false} {
			wg.Add(1)
			go func(i int, isA bool) {
				defer wg.Done()
				<-start
				if _, err := db.StoreMatchReportDetail(report(i, isA)); err != nil {
					errs <- err
				}
			}(i, isA)
		}
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	count := func(where string) int {
		var n int
		if err := db.pool.QueryRow(db.ctx, `SELECT count(*) FROM match_results WHERE pid_a >= $1 AND pid_a < $2 AND `+where,
			base, base+1000).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if c, all := count(`status = 'confirmed'`), count(`true`); c != pairs || all != pairs {
		t.Fatalf("%d simultaneous pairs -> %d confirmed of %d rows (%d single)", pairs, c, all, count(`status = 'single'`))
	}
	// Repeat reports: one side reporting twice adds no row; the other side then confirms the one row.
	r := report(pairs, true)
	for k := 0; k < 3; k++ {
		if res, err := db.StoreMatchReportDetail(r); err != nil || res.Status != "single" {
			t.Fatal(res, err)
		}
	}
	if n := count(`pid_a = ` + itoa(base+2*pairs)); n != 1 {
		t.Fatalf("3 reports from one side -> %d rows", n)
	}
	r.ReporterIsA = false
	if res, err := db.StoreMatchReportDetail(r); err != nil || res.Status != "confirmed" {
		t.Fatal(res, err)
	}
	if n := count(`pid_a = ` + itoa(base+2*pairs)); n != 1 {
		t.Fatalf("pair -> %d rows", n)
	}
	// Cap: a pair that keeps confirming new rows stops at MatchPairCapPerHour per hour.
	for k := 1; k < MatchPairCapPerHour; k++ {
		r.ReporterIsA = true
		if _, err := db.StoreMatchReportDetail(r); err != nil {
			t.Fatal(k, err)
		}
		r.ReporterIsA = false
		if res, err := db.StoreMatchReportDetail(r); err != nil || res.Status != "confirmed" {
			t.Fatal(k, res, err)
		}
	}
	r.ReporterIsA = true
	if _, err := db.StoreMatchReportDetail(r); !errors.Is(err, ErrPairCap) {
		t.Fatal("past the cap:", err)
	}
	if n := count(`pid_a = ` + itoa(base+2*pairs)); n != MatchPairCapPerHour {
		t.Fatalf("cap: %d rows", n)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
