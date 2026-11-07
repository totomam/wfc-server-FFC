package database

import (
	"errors"
	"os"
	"testing"

	"wwfc/common"
)

// Live Postgres check of the #53 links (skipped unless FFC_TEST_PG=1; local db wwfc/CHANGEME@127.0.0.1).
func TestPlayerLinksPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreatePlayerLinksTable(); err != nil {
		t.Fatal(err)
	}
	clean := func() {
		db.pool.Exec(db.ctx, `DELETE FROM ffc_player_links WHERE alias_pid BETWEEN 900000201 AND 900000209 OR canonical_pid BETWEEN 900000201 AND 900000209`) //nolint:errcheck
		db.pool.Exec(db.ctx, `DELETE FROM users WHERE profile_id BETWEEN 900000201 AND 900000209`)                                                            //nolint:errcheck
	}
	clean()
	defer clean()
	for i := 1; i <= 3; i++ {
		if _, err := db.pool.Exec(db.ctx, `INSERT INTO users (profile_id, user_id, gsbrcd, password, email, unique_nick) VALUES ($1, $2, 'VDEEJ', '', '', '')`,
			900000200+i, 7700000+i); err != nil {
			t.Fatal(err)
		}
	}
	// 202 -> 201; 203 -> 202 resolves to 201; 201 -> 203 resolves to 201 -> 201 = refused.
	if c, err := db.AddPlayerLink(900000202, 900000201, "old DS"); err != nil || c != 900000201 {
		t.Fatal(c, err)
	}
	if c, err := db.AddPlayerLink(900000203, 900000202, ""); err != nil || c != 900000201 {
		t.Fatal(c, err)
	}
	if _, err := db.AddPlayerLink(900000201, 900000203, ""); !errors.Is(err, ErrPlayerLinkInvalid) {
		t.Fatal("cycle not refused:", err)
	}
	if _, err := db.AddPlayerLink(900000209, 900000201, ""); !errors.Is(err, ErrPlayerLinkNoProfile) {
		t.Fatal("unknown profile not refused:", err)
	}
	m, err := db.PlayerLinks()
	if err != nil || m[900000202] != 900000201 || m[900000203] != 900000201 {
		t.Fatal(m, err)
	}
	// Re-point: make 201 an alias of a new canonical? Unlink 202, link 201 -> 202 (202 now plain): 203 follows to 202.
	if ok, err := db.DeletePlayerLink(900000202); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if c, err := db.AddPlayerLink(900000201, 900000202, ""); err != nil || c != 900000202 {
		t.Fatal(c, err)
	}
	m, _ = db.PlayerLinks()
	if m[900000201] != 900000202 || m[900000203] != 900000202 {
		t.Fatal("not re-pointed:", m)
	}
	ls, err := db.ListPlayerLinks()
	if err != nil || len(ls) < 2 {
		t.Fatal(ls, err)
	}
}
