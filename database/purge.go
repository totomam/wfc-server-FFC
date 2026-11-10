package database

import "fmt"

// Personal-data purge (NET-22, ffc-decomp issue #196). Rows older than the retention period lose what identifies a
// person or a device; the rows themselves (match results, events, profiles) stay for stats and moderation.
//   - match_results: report_a / report_b (raw console reports) -> NULL (created_at older than the cutoff)
//   - users: last_ip_address / last_ingamesn -> '' (last_ip_at older than the cutoff; the Go code scans them as
//     strings, so '' is the "no value" here). Profiles under an active ban keep their IP: login.go matches it to
//     stop ban evasion; it is purged once the ban has expired.
//   - events: event_data keys ip_address, ng_device_id, wii_number (console friend code), "Friend code" removed,
//     also inside "peers" (qr2 connection-failure events).
// pg_dump backups hold the same data until they age out (HOSTING.md §8: keep backups <= the retention period).

var purgeEventKeys = []string{"ip_address", "ng_device_id", "wii_number", "Friend code", "friend_code"}

const (
	purgeMatchReports = `UPDATE public.match_results SET report_a = NULL, report_b = NULL
		WHERE created_at < now() - make_interval(days => $1) AND (report_a IS NOT NULL OR report_b IS NOT NULL)`
	purgeUserIPs = `UPDATE public.users SET last_ip_address = '', last_ingamesn = ''
		WHERE last_ip_at < now() - make_interval(days => $1) AND (last_ip_address <> '' OR last_ingamesn <> '')
		AND NOT (COALESCE(has_ban, false) AND (ban_expires IS NULL OR ban_expires > now()))`
	purgeEvents = `UPDATE public.events SET event_data = CASE
			WHEN jsonb_typeof(event_data->'peers') = 'array' THEN jsonb_set(event_data - $2::text[], '{peers}',
				(SELECT COALESCE(jsonb_agg(CASE WHEN jsonb_typeof(p) = 'object' THEN p - $2::text[] ELSE p END), '[]'::jsonb)
				 FROM jsonb_array_elements(event_data->'peers') p))
			ELSE event_data - $2::text[] END
		WHERE event_time < now() - make_interval(days => $1)
		AND (event_data ?| $2::text[] OR (jsonb_typeof(event_data->'peers') = 'array' AND EXISTS (
			SELECT 1 FROM jsonb_array_elements(event_data->'peers') p WHERE jsonb_typeof(p) = 'object' AND p ?| $2::text[])))`
)

// PurgeResult counts the rows changed per table.
type PurgeResult struct {
	MatchResults, Users, Events int64
}

// PurgePersonalData clears personal data older than days (> 0) in one transaction.
func (c *Connection) PurgePersonalData(days int) (PurgeResult, error) {
	var r PurgeResult
	if days <= 0 {
		return r, fmt.Errorf("retention days must be > 0, got %d", days)
	}
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(c.ctx)
	tag, err := tx.Exec(c.ctx, purgeMatchReports, days)
	if err != nil {
		return r, fmt.Errorf("match_results: %w", err)
	}
	r.MatchResults = tag.RowsAffected()
	if tag, err = tx.Exec(c.ctx, purgeUserIPs, days); err != nil {
		return r, fmt.Errorf("users: %w", err)
	}
	r.Users = tag.RowsAffected()
	if tag, err = tx.Exec(c.ctx, purgeEvents, days, purgeEventKeys); err != nil {
		return r, fmt.Errorf("events: %w", err)
	}
	r.Events = tag.RowsAffected()
	return r, tx.Commit(c.ctx)
}
