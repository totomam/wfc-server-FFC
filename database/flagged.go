package database

import "time"

// FFC admin view of match_results rows that need a moderator's eye (ffc-decomp ROADMAP #28): "mismatch" = the two
// consoles' reports of one match disagree (field in Mismatch); "single" = only one console reported (the other one
// disconnected, runs a stock ROM, or never sent). Newest first. Raw reports stay in the table (report_a/report_b).
const queryFlaggedRows = `SELECT id, pid_a, pid_b, COALESCE(winner_pid, 0), draw, time_up, modded, match_ver, name_a, name_b,
status, mismatch, report_a IS NOT NULL, report_b IS NOT NULL, created_at, updated_at FROM match_results
WHERE status = ANY($1) AND created_at >= $2 ORDER BY created_at DESC, id DESC LIMIT $3`

type FlaggedMatch struct {
	ID        int64     `json:"id"`
	PidA      uint32    `json:"pid_a"`
	PidB      uint32    `json:"pid_b"`
	WinnerPid uint32    `json:"winner_pid"`
	Draw      bool      `json:"draw"`
	TimeUp    bool      `json:"time_up"`
	Modded    bool      `json:"modded"`
	MatchVer  uint32    `json:"match_ver"`
	NameA     string    `json:"name_a"`
	NameB     string    `json:"name_b"`
	Status    string    `json:"status"`
	Mismatch  string    `json:"mismatch"`
	ReportedA bool      `json:"reported_a"`
	ReportedB bool      `json:"reported_b"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Connection) LoadFlaggedMatches(statuses []string, since time.Time, limit int) ([]FlaggedMatch, error) {
	if since.IsZero() {
		since = time.Unix(0, 0).UTC()
	}
	rows, err := c.pool.Query(c.ctx, queryFlaggedRows, statuses, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FlaggedMatch{}
	for rows.Next() {
		var (
			m          FlaggedMatch
			a, b, w, v int64
		)
		if err := rows.Scan(&m.ID, &a, &b, &w, &m.Draw, &m.TimeUp, &m.Modded, &v, &m.NameA, &m.NameB, &m.Status,
			&m.Mismatch, &m.ReportedA, &m.ReportedB, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.PidA, m.PidB, m.WinnerPid, m.MatchVer = uint32(a), uint32(b), uint32(w), uint32(v)
		out = append(out, m)
	}
	return out, rows.Err()
}

// FlaggedCounts sums mismatch rows per profile (both sides of a mismatch are counted: the server cannot tell which
// console lied; a profile that keeps showing up is the one to look at).
func FlaggedCounts(ms []FlaggedMatch) map[uint32]int {
	n := map[uint32]int{}
	for _, m := range ms {
		if m.Status == "mismatch" {
			n[m.PidA]++
			n[m.PidB]++
		}
	}
	return n
}
