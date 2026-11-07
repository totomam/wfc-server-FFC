package database

import (
	"os"
	"reflect"
	"testing"

	"wwfc/common"
)

// Live Postgres (FFC_TEST_PG=1): ProfilesOfUser returns only that console's FFC profiles, oldest first.
func TestProfilesOfUserPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	ex := func(q string, a ...any) {
		if _, err := db.pool.Exec(db.ctx, q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`DELETE FROM users WHERE profile_id >= 900000200 AND profile_id < 900000210`)
	defer ex(`DELETE FROM users WHERE profile_id >= 900000200 AND profile_id < 900000210`)
	add := `INSERT INTO users (profile_id, user_id, gsbrcd, password, email, unique_nick) VALUES ($1, $2, $3, '', '', '')`
	ex(add, 900000203, 77701, "VDEEJabcd")
	ex(add, 900000201, 77701, "VDEEJefgh")
	ex(add, 900000202, 77701, "RMCJxxxx") // another game on the same console
	ex(add, 900000204, 77702, "VDEEJzzzz")
	got, err := db.ProfilesOfUser(77701, "VDEE")
	if err != nil || !reflect.DeepEqual(got, []uint32{900000201, 900000203}) {
		t.Fatal(got, err)
	}
	if got, err := db.ProfilesOfUser(1, "VDEE"); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
}
