package database

import (
	"os"
	"testing"
	"wwfc/common"
)

// NET-22 live check (FFC_TEST_PG=1; local db wwfc/CHANGEME@127.0.0.1): rows older than the retention period lose
// reports / IPs / device IDs / friend codes, newer rows and actively banned users keep them.
func TestPurgePersonalDataPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	db.UpdateTables()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	ex := func(q string, a ...any) {
		t.Helper()
		if _, err := db.pool.Exec(db.ctx, q, a...); err != nil {
			t.Fatal(q, err)
		}
	}
	const p0 = 990000100
	clean := func() {
		ex(`DELETE FROM match_results WHERE pid_a BETWEEN $1 AND $1 + 9`, p0)
		ex(`DELETE FROM users WHERE profile_id BETWEEN $1 AND $1 + 9`, p0)
		ex(`DELETE FROM events WHERE event_type = 'net22_test'`)
	}
	clean()
	defer clean()
	for i, age := range []int{200, 1} { // p0: 200 days old, p0+1: 1 day old
		pid := p0 + i
		ex(`INSERT INTO match_results (pid_a, pid_b, team_a, team_b, report_a, report_b, created_at)
			VALUES ($1, $1, '[]', '[]', '\x01', '\x02', now() - make_interval(days => $2))`, pid, age)
		ex(`INSERT INTO users (profile_id, user_id, gsbrcd, password, email, unique_nick, last_ip_address, last_ingamesn, last_ip_at)
			VALUES ($1, $1, 'VDEEx', '', '', '', '203.0.113.9', 'Hunter', now() - make_interval(days => $2))`, pid, age)
		ex(`INSERT INTO events (event_type, event_data, event_time) VALUES ('net22_test',
			jsonb_build_object('profile_id', $1::bigint, 'ip_address', '203.0.113.9', 'ng_device_id', 42, 'Friend code', '1234-5678-9012',
				'peers', jsonb_build_array(jsonb_build_object('profile_id', '1', 'ip_address', '198.51.100.1'))),
			now() - make_interval(days => $2))`, pid, age)
	}
	// p0+2: old but under an active ban: keeps its IP (ban-evasion check in login.go).
	ex(`INSERT INTO users (profile_id, user_id, gsbrcd, password, email, unique_nick, last_ip_address, last_ingamesn, last_ip_at,
		has_ban, ban_expires) VALUES ($1, $1, 'VDEEx', '', '', '', '203.0.113.10', 'Banned', now() - interval '200 days', true,
		now() + interval '30 days')`, p0+2)

	r, err := db.PurgePersonalData(90)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("purged %+v", r)

	var oldNull, newNull bool
	db.pool.QueryRow(db.ctx, `SELECT report_a IS NULL AND report_b IS NULL FROM match_results WHERE pid_a = $1`, p0).Scan(&oldNull)
	db.pool.QueryRow(db.ctx, `SELECT report_a IS NULL OR report_b IS NULL FROM match_results WHERE pid_a = $1`, p0+1).Scan(&newNull)
	if !oldNull || newNull {
		t.Fatalf("match_results: old cleared %v, new cleared %v", oldNull, newNull)
	}
	ip := func(pid int) (s string) {
		db.pool.QueryRow(db.ctx, `SELECT last_ip_address || '|' || last_ingamesn FROM users WHERE profile_id = $1`, pid).Scan(&s)
		return
	}
	if ip(p0) != "|" || ip(p0+1) != "203.0.113.9|Hunter" || ip(p0+2) != "203.0.113.10|Banned" {
		t.Fatalf("users: %q %q %q", ip(p0), ip(p0+1), ip(p0+2))
	}
	ev := func(pid int) (s string) {
		db.pool.QueryRow(db.ctx, `SELECT event_data::text FROM events WHERE event_type = 'net22_test' AND (event_data->>'profile_id')::bigint = $1`, pid).Scan(&s)
		return
	}
	var leftOld int
	db.pool.QueryRow(db.ctx, `SELECT count(*) FROM events WHERE event_type = 'net22_test' AND (event_data->>'profile_id')::bigint = $1
		AND (event_data->>'ip_address' IS NOT NULL OR event_data->>'ng_device_id' IS NOT NULL OR event_data->>'Friend code' IS NOT NULL
		OR event_data->'peers'->0->>'ip_address' IS NOT NULL)`, p0).Scan(&leftOld)
	if leftOld != 0 || ev(p0) == "" {
		t.Fatalf("old event still has personal data: %s", ev(p0))
	}
	var keptNew int
	db.pool.QueryRow(db.ctx, `SELECT count(*) FROM events WHERE event_type = 'net22_test' AND (event_data->>'profile_id')::bigint = $1
		AND event_data->>'ip_address' = '203.0.113.9' AND event_data->'peers'->0->>'ip_address' = '198.51.100.1'`, p0+1).Scan(&keptNew)
	if keptNew != 1 {
		t.Fatalf("new event changed: %s", ev(p0+1))
	}
	t.Log("old event after purge:", ev(p0))
	if r2, err := db.PurgePersonalData(90); err != nil || r2 != (PurgeResult{}) {
		t.Fatal("second run not a no-op:", r2, err)
	}
}
