package database

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

// PvP match results (FFC, ffc-decomp ROADMAP #20). Both consoles of a match report (nas/dls_result.go, Nd action
// "result"); the first report creates the row (status "single"), the other side's report fills its column and
// cross-checks the outcome (#28): "confirmed" when both agree, "mismatch" otherwise (both raw reports kept).
// Wire format: docs/sessions/s95-online/payload_spec.md (ffc-decomp).

const (
	createMatchResults = `CREATE TABLE IF NOT EXISTS public.match_results (
    id bigserial PRIMARY KEY,
    pid_a bigint NOT NULL,
    pid_b bigint NOT NULL,
    winner_pid bigint,
    draw boolean NOT NULL DEFAULT false,
    time_up boolean NOT NULL DEFAULT false,
    modded boolean NOT NULL DEFAULT false,
    match_ver bigint NOT NULL DEFAULT 0,
    name_a character varying NOT NULL DEFAULT '',
    name_b character varying NOT NULL DEFAULT '',
    team_a jsonb NOT NULL,
    team_b jsonb NOT NULL,
    report_a bytea,
    report_b bytea,
    status character varying NOT NULL DEFAULT 'single',
    mismatch character varying NOT NULL DEFAULT '',
    queue character varying NOT NULL DEFAULT '',
    created_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS match_results_pair_idx ON public.match_results (pid_a, pid_b, created_at);
-- s99 #35: tables made before the queue column
ALTER TABLE public.match_results ADD COLUMN IF NOT EXISTS queue character varying NOT NULL DEFAULT '';
-- NET-11: the one open (waiting for the other console) row of a pair; the partial unique index makes pairing atomic.
ALTER TABLE public.match_results ADD COLUMN IF NOT EXISTS pair_open boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS match_results_open_pair_idx ON public.match_results (pid_a, pid_b) WHERE pair_open;
-- NET-35: who sent the first report, and whether the pair had been seen in one QR2 group (NET-10) when it was stored;
-- a single row with pair_seen counts as a loss for the other side after the forfeit timeout (ffcstats.ApplyForfeits).
ALTER TABLE public.match_results ADD COLUMN IF NOT EXISTS reporter_pid bigint NOT NULL DEFAULT 0;
ALTER TABLE public.match_results ADD COLUMN IF NOT EXISTS pair_seen boolean NOT NULL DEFAULT false`

	queryUserOwnsProfile = `SELECT EXISTS(SELECT 1 FROM users WHERE profile_id = $1 AND user_id = $2)`
	queryProfileExists   = `SELECT EXISTS(SELECT 1 FROM users WHERE profile_id = $1)`

	// NET-11: a pair has at most one open row (pair_open, unique). It closes when the other side's report completes it
	// or once it is matchPairWindow old (closed by the next report of the pair; the row stays "single").
	queryCloseStaleOpen = `UPDATE match_results SET pair_open = false
WHERE pid_a = $1 AND pid_b = $2 AND pair_open AND created_at <= CURRENT_TIMESTAMP - interval '10 minutes'`
	queryFindOpenMatch = `SELECT id, winner_pid, draw, time_up, match_ver, team_a, team_b, report_a IS NOT NULL, report_b IS NOT NULL
FROM match_results WHERE pid_a = $1 AND pid_b = $2 AND pair_open FOR UPDATE`
	queryCountPairRows = `SELECT count(*) FROM match_results WHERE pid_a = $1 AND pid_b = $2
AND created_at > CURRENT_TIMESTAMP - interval '1 hour'`
	queryInsertMatch = `INSERT INTO match_results (pid_a, pid_b, winner_pid, draw, time_up, modded, match_ver, name_a, name_b,
team_a, team_b, report_a, report_b, status, queue, pair_open, reporter_pid, pair_seen)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'single', $14, true, $15, $16)
ON CONFLICT (pid_a, pid_b) WHERE pair_open DO NOTHING RETURNING id`
	queryCompleteMatch = `UPDATE match_results SET report_a = COALESCE(report_a, $2), report_b = COALESCE(report_b, $3),
status = $4, mismatch = $5, queue = CASE WHEN queue = '' THEN $6 ELSE queue END, time_up = time_up AND $7,
pair_open = false, updated_at = CURRENT_TIMESTAMP WHERE id = $1`
)

// MatchReport is one console's validated report, already normalised to the pair order (A = lower profile id).
type MatchReport struct {
	PidA, PidB   uint32
	WinnerPid    uint32 // 0 = draw
	Draw         bool
	TimeUp       bool
	Modded       bool
	MatchVer     uint32
	NameA, NameB string
	TeamA, TeamB json.RawMessage
	ReporterIsA  bool
	// Queue (s99 #35): "ranked" = Battle with Anyone (QR2 group match type anybody), "casual" = friend battle
	// (private), "" = unknown (no group found at report time; counted as ranked).
	Queue string
	Raw   []byte
	// PairSeen (NET-10): the server saw the two profiles in one QR2 group before this report (nas checks it).
	PairSeen bool
}

func (c *Connection) CreateMatchResultsTable() error {
	_, err := c.pool.Exec(c.ctx, createMatchResults)
	return err
}

func (c *Connection) UserOwnsProfile(userID uint64, profileID uint32) (bool, error) {
	var ok bool
	err := c.pool.QueryRow(c.ctx, queryUserOwnsProfile, profileID, userID).Scan(&ok)
	return ok, err
}

func (c *Connection) ProfileExists(profileID uint32) (bool, error) {
	var ok bool
	err := c.pool.QueryRow(c.ctx, queryProfileExists, profileID).Scan(&ok)
	return ok, err
}

func nullPid(p uint32) any {
	if p == 0 {
		return nil
	}
	return int64(p)
}

// StoreResult is what one stored report did to its row: id, status ("single", "confirmed", "mismatch") and, for a
// mismatch, the first field the two reports disagree on (CompareMatch).
type StoreResult struct {
	ID       int64
	Status   string
	Mismatch string
}

// StoreMatchReport stores one side's report: it completes the open row of the same pair (cross-check) or creates
// a new row. Returns the row id and its status ("single", "confirmed", "mismatch").
func (c *Connection) StoreMatchReport(r MatchReport) (int64, string, error) {
	res, err := c.StoreMatchReportDetail(r)
	return res.ID, res.Status, err
}

// MatchPairCapPerHour (NET-11): at most this many rows per pair (pid_a, pid_b) per rolling hour; past it a report
// that would open a new row is refused with ErrPairCap (completing the open row is always allowed).
const MatchPairCapPerHour = 30

var ErrPairCap = errors.New("match_results: too many rows for this pair in the last hour")

// StoreMatchReportDetail is StoreMatchReport plus the mismatch field (for the #28 alert).
// NET-11: pairing is atomic. The insert of a new open row conflicts with the pair's open row (partial unique index),
// so two simultaneous reports of one match end in one row: the loser of the insert race completes the winner's row.
// A repeated report from the side that already reported adds no row (the open row is returned unchanged).
func (c *Connection) StoreMatchReportDetail(r MatchReport) (StoreResult, error) {
	for attempt := 0; attempt < 3; attempt++ {
		res, retry, err := c.storeMatchReportOnce(r)
		if !retry {
			return res, err
		}
	}
	return StoreResult{}, fmt.Errorf("match_results: pair %d-%d: open row kept changing", r.PidA, r.PidB)
}

// storeMatchReportOnce: retry = the open row the insert conflicted with was closed before it could be read.
func (c *Connection) storeMatchReportOnce(r MatchReport) (StoreResult, bool, error) {
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		return StoreResult{}, false, err
	}
	defer func() { _ = tx.Rollback(c.ctx) }()

	var (
		id           int64
		winner       *int64
		draw         bool
		timeUp       bool
		matchVer     int64
		teamA, teamB []byte
		hasA, hasB   bool
		repA, repB   []byte
	)
	if r.ReporterIsA {
		repA = r.Raw
	} else {
		repB = r.Raw
	}
	pa, pb := int64(r.PidA), int64(r.PidB)
	if _, err = tx.Exec(c.ctx, queryCloseStaleOpen, pa, pb); err != nil {
		return StoreResult{}, false, err
	}
	err = tx.QueryRow(c.ctx, queryFindOpenMatch, pa, pb).Scan(&id, &winner, &draw, &timeUp, &matchVer, &teamA, &teamB, &hasA, &hasB)
	if errors.Is(err, pgx.ErrNoRows) {
		var n int
		if err = tx.QueryRow(c.ctx, queryCountPairRows, pa, pb).Scan(&n); err != nil {
			return StoreResult{}, false, err
		}
		if n >= MatchPairCapPerHour {
			return StoreResult{}, false, ErrPairCap
		}
		err = tx.QueryRow(c.ctx, queryInsertMatch, pa, pb, nullPid(r.WinnerPid), r.Draw, r.TimeUp, r.Modded, int64(r.MatchVer),
			r.NameA, r.NameB, []byte(r.TeamA), []byte(r.TeamB), repA, repB, r.Queue, int64(r.reporterPid()), r.PairSeen).Scan(&id)
		if err == nil {
			return StoreResult{ID: id, Status: "single"}, false, tx.Commit(c.ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return StoreResult{}, false, err
		}
		// Lost the insert race: the other console's row committed meanwhile (READ COMMITTED: a new statement sees it).
		err = tx.QueryRow(c.ctx, queryFindOpenMatch, pa, pb).Scan(&id, &winner, &draw, &timeUp, &matchVer, &teamA, &teamB, &hasA, &hasB)
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreResult{}, true, nil
		}
	}
	if err != nil {
		return StoreResult{}, false, err
	}
	if (r.ReporterIsA && hasA) || (!r.ReporterIsA && hasB) {
		return StoreResult{ID: id, Status: "single"}, false, nil // repeat from the side that already reported: no new row
	}
	mismatch := CompareMatch(r, winner, draw, timeUp, uint32(matchVer), teamA, teamB)
	status := "confirmed"
	if mismatch != "" {
		status = "mismatch"
	}
	if _, err = tx.Exec(c.ctx, queryCompleteMatch, id, repA, repB, status, mismatch, r.Queue, r.TimeUp); err != nil {
		return StoreResult{}, false, err
	}
	return StoreResult{ID: id, Status: status, Mismatch: mismatch}, false, tx.Commit(c.ctx)
}

func (r MatchReport) reporterPid() uint32 {
	if r.ReporterIsA {
		return r.PidA
	}
	return r.PidB
}

// CompareMatch returns "" when the second report agrees with the stored row, else the first differing field.
// time_up is not compared (s119-online): the game sets the console's scene+0x6b8 byte on a time-limit end but never
// clears it, so a console can report a stale 1; the row keeps time_up only when both reports say so (AND).
func CompareMatch(r MatchReport, winner *int64, draw, _ bool, matchVer uint32, teamA, teamB []byte) string {
	var w uint32
	if winner != nil {
		w = uint32(*winner)
	}
	switch {
	case w != r.WinnerPid || draw != r.Draw:
		return "winner"
	case matchVer != r.MatchVer:
		return "match_ver"
	case !jsonEqual(teamA, r.TeamA):
		return "team_a"
	case !jsonEqual(teamB, r.TeamB):
		return "team_b"
	}
	return ""
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

// ProfilesOfUser: the profiles of one NAS user id (console) whose gsbrcd starts with gamecd (FFC "VDEE"), oldest
// first (#76 news: the Bonus Data request carries only the userid).
func (c *Connection) ProfilesOfUser(userID uint64, gamecd string) ([]uint32, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT profile_id FROM users WHERE user_id = $1 AND gsbrcd LIKE $2 || '%' ORDER BY profile_id`, userID, gamecd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uint32
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, uint32(p))
	}
	return out, rows.Err()
}
