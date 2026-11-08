package database

import "time"

// FFC leaderboard names (ffc-decomp ROADMAP #52, s113-online): the name a player typed in game (Nd action "lbname",
// nas/dls_lbname.go) replaces the save's player name in every /api/ffc stats view + the widget (ffcstats.Names).
// One row per profile; the console's request carries only its NAS user id, so a set writes every FFC profile of
// that user id.

const ffcLbNamesTable = `
CREATE TABLE IF NOT EXISTS public.ffc_lb_names (
	profile_id bigint PRIMARY KEY,
	name character varying NOT NULL CHECK (LENGTH(name) BETWEEN 1 AND 16),
	updated_at timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC')
);`

func (c *Connection) CreateLbNamesTable() error {
	_, err := c.pool.Exec(c.ctx, ffcLbNamesTable)
	return err
}

// SetLbName stores name for each profile in pids (insert or replace).
func (c *Connection) SetLbName(pids []uint32, name string) error {
	for _, p := range pids {
		if _, err := c.pool.Exec(c.ctx, `INSERT INTO ffc_lb_names (profile_id, name, updated_at) VALUES ($1, $2, $3)
			ON CONFLICT (profile_id) DO UPDATE SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at`,
			int64(p), name, time.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

// LbNameOf: the name of the first profile in pids that has one ("" = none) and the profiles in pids without one.
func (c *Connection) LbNameOf(pids []uint32) (string, []uint32, error) {
	ids := make([]int64, len(pids))
	for i, p := range pids {
		ids[i] = int64(p)
	}
	rows, err := c.pool.Query(c.ctx, `SELECT profile_id, name FROM ffc_lb_names WHERE profile_id = ANY($1)`, ids)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	have := map[uint32]string{}
	for rows.Next() {
		var p int64
		var n string
		if err := rows.Scan(&p, &n); err != nil {
			return "", nil, err
		}
		have[uint32(p)] = n
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	name, missing := "", []uint32{}
	for _, p := range pids {
		if n, ok := have[p]; !ok {
			missing = append(missing, p)
		} else if name == "" {
			name = n
		}
	}
	return name, missing, nil
}

// LbNames: profile id -> leaderboard name.
func (c *Connection) LbNames() (map[uint32]string, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT profile_id, name FROM ffc_lb_names`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]string{}
	for rows.Next() {
		var p int64
		var n string
		if err := rows.Scan(&p, &n); err != nil {
			return nil, err
		}
		out[uint32(p)] = n
	}
	return out, rows.Err()
}

// DeleteLbName removes one profile's name (admin moderation); false = no row.
func (c *Connection) DeleteLbName(pid uint32) (bool, error) {
	tag, err := c.pool.Exec(c.ctx, `DELETE FROM ffc_lb_names WHERE profile_id = $1`, int64(pid))
	return tag.RowsAffected() > 0, err
}
