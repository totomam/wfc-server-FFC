package database

import (
	"time"

	"wwfc/ffcstats"
)

// FFC leaderboard source (ffc-decomp ROADMAP #21-#27, #32): match_results rows in a time window, any status
// (ffcstats.Filter picks the statuses). Stats are computed on read; at FFC's scale a full replay is cheap.
const queryMatchRows = `SELECT id, pid_a, pid_b, COALESCE(winner_pid, 0), draw, time_up, modded, match_ver, name_a, name_b,
team_a, team_b, status, created_at FROM match_results
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
			teamA, teamB []byte
		)
		if err := rows.Scan(&m.ID, &a, &b, &w, &m.Draw, &m.TimeUp, &m.Modded, &v, &m.NameA, &m.NameB, &teamA, &teamB,
			&m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.PidA, m.PidB, m.WinnerPid, m.MatchVer = uint32(a), uint32(b), uint32(w), uint32(v)
		m.TeamA, m.TeamB = ffcstats.ParseTeam(teamA), ffcstats.ParseTeam(teamB)
		out = append(out, m)
	}
	return out, rows.Err()
}
