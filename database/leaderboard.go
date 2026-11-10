package database

import (
	"sync/atomic"
	"time"

	"wwfc/ffcstats"
)

// FFC leaderboard source (ffc-decomp ROADMAP #21-#27, #32): match_results rows in a time window, any status
// (ffcstats.Filter picks the statuses). Stats are computed on read; at FFC's scale a full replay is cheap.
const queryMatchRows = `SELECT id, pid_a, pid_b, COALESCE(winner_pid, 0), draw, time_up, modded, match_ver, name_a, name_b,
team_a, team_b, status, queue, created_at, reporter_pid, pair_seen FROM match_results
WHERE created_at >= $1 AND created_at < $2 ORDER BY created_at, id LIMIT $3`

func (c *Connection) LoadMatches(from, to time.Time, limit int) ([]ffcstats.Match, error) {
	if from.IsZero() {
		from = time.Unix(0, 0).UTC()
	}
	if to.IsZero() {
		to = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	rows, err := c.pool.Query(c.ctx, queryMatchRows, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ffcstats.Match{}
	for rows.Next() {
		var (
			m            ffcstats.Match
			a, b, w, v   int64
			rep          int64
			teamA, teamB []byte
		)
		if err := rows.Scan(&m.ID, &a, &b, &w, &m.Draw, &m.TimeUp, &m.Modded, &v, &m.NameA, &m.NameB, &teamA, &teamB,
			&m.Status, &m.Queue, &m.CreatedAt, &rep, &m.PairSeen); err != nil {
			return nil, err
		}
		m.PidA, m.PidB, m.WinnerPid, m.MatchVer, m.Reporter = uint32(a), uint32(b), uint32(w), uint32(v), uint32(rep)
		m.TeamA, m.TeamB = ffcstats.ParseTeam(teamA), ffcstats.ParseTeam(teamB)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ffcstats.ApplyForfeits(out, time.Duration(forfeitAfter.Load()), time.Now().UTC()), nil
}

// DefaultForfeitAfter (NET-35): a "single" row (one console reported) this old is a loss for the side that did not
// report (ffcstats.ApplyForfeits). 15 min: above the 10-minute window in which the second report can still complete
// the row (queryCloseStaleOpen), well above a console's report delay (it posts when the battle screen ends).
const DefaultForfeitAfter = 15 * time.Minute

// forfeitAfter is DefaultForfeitAfter or config.xml <resultForfeitMinutes> (SetForfeitMinutes); <= 0 = never.
var forfeitAfter atomic.Int64

func init() { forfeitAfter.Store(int64(DefaultForfeitAfter)) }

// SetForfeitMinutes applies config.xml <resultForfeitMinutes> (nil = keep the default; 0 or less = never).
func SetForfeitMinutes(m *int) {
	if m != nil {
		forfeitAfter.Store(int64(time.Duration(*m) * time.Minute))
	}
}
