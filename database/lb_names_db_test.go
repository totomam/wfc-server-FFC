package database

import (
	"os"
	"testing"

	"wwfc/common"
)

// Live Postgres check of the #52 leaderboard names (skipped unless FFC_TEST_PG=1; local db wwfc/CHANGEME@127.0.0.1).
func TestLbNamesPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateLbNamesTable(); err != nil {
		t.Fatal(err)
	}
	clean := func() {
		db.pool.Exec(db.ctx, `DELETE FROM ffc_lb_names WHERE profile_id BETWEEN 900000301 AND 900000309`) //nolint:errcheck
	}
	clean()
	defer clean()
	if n, miss, err := db.LbNameOf([]uint32{900000301, 900000302}); err != nil || n != "" || len(miss) != 2 {
		t.Fatalf("empty: %q %v %v", n, miss, err)
	}
	if err := db.SetLbName([]uint32{900000301}, "Rex"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetLbName([]uint32{900000301}, "Rex Fan"); err != nil {
		t.Fatal(err)
	}
	n, miss, err := db.LbNameOf([]uint32{900000301, 900000302})
	if err != nil || n != "Rex Fan" || len(miss) != 1 || miss[0] != 900000302 {
		t.Fatalf("after set: %q %v %v", n, miss, err)
	}
	all, err := db.LbNames()
	if err != nil || all[900000301] != "Rex Fan" {
		t.Fatalf("LbNames: %v %v", all, err)
	}
	if ok, err := db.DeleteLbName(900000301); err != nil || !ok {
		t.Fatalf("delete: %v %v", ok, err)
	}
	if ok, _ := db.DeleteLbName(900000301); ok {
		t.Fatal("second delete found a row")
	}
}
