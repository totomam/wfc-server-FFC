package database

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v4"
)

// FFC persistent player identity (ffc-decomp ROADMAP #53). The server keys a player on profile_id, which is
// bound to (user_id, gsbrcd): user_id lives in the console's WFC settings (firmware), the profile id in the
// save's DWC user data. A new console, a wiped WFC config or a fresh emulator firmware gives a new profile id,
// so one person can end up with several. An admin links each extra ("alias") profile to the one that keeps
// the record ("canonical"); the FFC stats (/api/ffc/*) then count an alias's matches as the canonical's.
// match_results rows are never rewritten: unlinking restores the old view. Links are one level deep: linking
// to an alias resolves to that alias's canonical, and links that pointed at a new alias are re-pointed.

const ffcPlayerLinksTable = `
CREATE TABLE IF NOT EXISTS public.ffc_player_links (
	alias_pid bigint PRIMARY KEY,
	canonical_pid bigint NOT NULL,
	note character varying NOT NULL DEFAULT '' CHECK (LENGTH(note) <= 64),
	created_at timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
	CHECK (alias_pid <> canonical_pid)
);
CREATE INDEX IF NOT EXISTS ffc_player_links_canonical_idx ON public.ffc_player_links (canonical_pid);`

var (
	ErrPlayerLinkInvalid   = errors.New("invalid player link")
	ErrPlayerLinkNoProfile = errors.New("profile not found")
)

type PlayerLink struct {
	AliasPid     uint32    `json:"alias_pid"`
	CanonicalPid uint32    `json:"canonical_pid"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
}

func (c *Connection) CreatePlayerLinksTable() error {
	_, err := c.pool.Exec(c.ctx, ffcPlayerLinksTable)
	return err
}

// PlayerLinks: alias -> canonical, for ffcstats.ApplyLinks.
func (c *Connection) PlayerLinks() (map[uint32]uint32, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT alias_pid, canonical_pid FROM ffc_player_links`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]uint32{}
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out[uint32(a)] = uint32(b)
	}
	return out, rows.Err()
}

func (c *Connection) ListPlayerLinks() ([]PlayerLink, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT alias_pid, canonical_pid, note, created_at FROM ffc_player_links ORDER BY canonical_pid, alias_pid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlayerLink{}
	for rows.Next() {
		var l PlayerLink
		var a, b int64
		if err := rows.Scan(&a, &b, &l.Note, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.AliasPid, l.CanonicalPid = uint32(a), uint32(b)
		out = append(out, l)
	}
	return out, rows.Err()
}

// AddPlayerLink links alias to canonical (both must be known profiles) and returns the canonical actually used.
// Re-linking an alias replaces its old link.
func (c *Connection) AddPlayerLink(alias, canonical uint32, note string) (uint32, error) {
	if alias == 0 || canonical == 0 || len(note) > 64 {
		return 0, ErrPlayerLinkInvalid
	}
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(c.ctx) //nolint:errcheck
	for _, pid := range []uint32{alias, canonical} {
		var ok bool
		if err := tx.QueryRow(c.ctx, queryProfileExists, pid).Scan(&ok); err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrPlayerLinkNoProfile
		}
	}
	var up int64
	switch err := tx.QueryRow(c.ctx, `SELECT canonical_pid FROM ffc_player_links WHERE alias_pid = $1`, canonical).Scan(&up); {
	case err == nil:
		canonical = uint32(up)
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, err
	}
	if canonical == alias {
		return 0, ErrPlayerLinkInvalid
	}
	if _, err := tx.Exec(c.ctx, `UPDATE ffc_player_links SET canonical_pid = $2 WHERE canonical_pid = $1`, alias, canonical); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(c.ctx, `INSERT INTO ffc_player_links (alias_pid, canonical_pid, note) VALUES ($1, $2, $3)
		ON CONFLICT (alias_pid) DO UPDATE SET canonical_pid = EXCLUDED.canonical_pid, note = EXCLUDED.note, created_at = now() AT TIME ZONE 'UTC'`,
		alias, canonical, note); err != nil {
		return 0, err
	}
	return canonical, tx.Commit(c.ctx)
}

func (c *Connection) DeletePlayerLink(alias uint32) (bool, error) {
	tag, err := c.pool.Exec(c.ctx, `DELETE FROM ffc_player_links WHERE alias_pid = $1`, alias)
	return tag.RowsAffected() > 0, err
}
