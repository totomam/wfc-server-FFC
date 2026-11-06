package database

import (
	"bytes"
	"encoding/json"
	"errors"

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
    created_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS match_results_pair_idx ON public.match_results (pid_a, pid_b, created_at)`

	queryUserOwnsProfile = `SELECT EXISTS(SELECT 1 FROM users WHERE profile_id = $1 AND user_id = $2)`
	queryProfileExists   = `SELECT EXISTS(SELECT 1 FROM users WHERE profile_id = $1)`

	// The other side's open row of the same pair, at most matchPairWindow old, still missing this reporter's column.
	queryFindOpenMatch = `SELECT id, winner_pid, draw, time_up, match_ver, team_a, team_b FROM match_results
WHERE pid_a = $1 AND pid_b = $2 AND created_at > CURRENT_TIMESTAMP - interval '10 minutes'
  AND status = 'single' AND (CASE WHEN $3 THEN report_a IS NULL ELSE report_b IS NULL END)
ORDER BY created_at DESC LIMIT 1 FOR UPDATE`
	queryInsertMatch = `INSERT INTO match_results (pid_a, pid_b, winner_pid, draw, time_up, modded, match_ver, name_a, name_b,
team_a, team_b, report_a, report_b, status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'single') RETURNING id`
	queryCompleteMatch = `UPDATE match_results SET report_a = COALESCE(report_a, $2), report_b = COALESCE(report_b, $3),
status = $4, mismatch = $5, updated_at = CURRENT_TIMESTAMP WHERE id = $1`
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
	Raw          []byte
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

// StoreMatchReport stores one side's report: it completes the open row of the same pair (cross-check) or creates
// a new row. Returns the row id and its status ("single", "confirmed", "mismatch").
func (c *Connection) StoreMatchReport(r MatchReport) (int64, string, error) {
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback(c.ctx) }()

	var (
		id        int64
		winner    *int64
		draw      bool
		timeUp    bool
		matchVer  int64
		teamA     []byte
		teamB     []byte
		repA      []byte
		repB      []byte
		status    = "single"
		mismatch  string
		reportCol = r.Raw
	)
	if r.ReporterIsA {
		repA = reportCol
	} else {
		repB = reportCol
	}

	err = tx.QueryRow(c.ctx, queryFindOpenMatch, int64(r.PidA), int64(r.PidB), r.ReporterIsA).
		Scan(&id, &winner, &draw, &timeUp, &matchVer, &teamA, &teamB)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = tx.QueryRow(c.ctx, queryInsertMatch, int64(r.PidA), int64(r.PidB), nullPid(r.WinnerPid), r.Draw, r.TimeUp,
			r.Modded, int64(r.MatchVer), r.NameA, r.NameB, []byte(r.TeamA), []byte(r.TeamB), repA, repB).Scan(&id)
		if err != nil {
			return 0, "", err
		}
	case err != nil:
		return 0, "", err
	default:
		mismatch = CompareMatch(r, winner, draw, timeUp, uint32(matchVer), teamA, teamB)
		status = "confirmed"
		if mismatch != "" {
			status = "mismatch"
		}
		if _, err = tx.Exec(c.ctx, queryCompleteMatch, id, repA, repB, status, mismatch); err != nil {
			return 0, "", err
		}
	}
	return id, status, tx.Commit(c.ctx)
}

// CompareMatch returns "" when the second report agrees with the stored row, else the first differing field.
func CompareMatch(r MatchReport, winner *int64, draw, timeUp bool, matchVer uint32, teamA, teamB []byte) string {
	var w uint32
	if winner != nil {
		w = uint32(*winner)
	}
	switch {
	case w != r.WinnerPid || draw != r.Draw:
		return "winner"
	case timeUp != r.TimeUp:
		return "time_up"
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
