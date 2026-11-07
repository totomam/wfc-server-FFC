package database

import (
	"errors"
	"time"
)

// FFC event / boss Plaza teams (ffc-decomp ROADMAP #40). A row schedules one sake record (a ghost team made with
// decomp tools/online_ghost.py or the admin panel) for a UTC window. A record that has at least one row here is
// visible to the game (SearchForRecords / GetRandomRecords / GetSpecificRecords, all through GetSakeRecords) only
// while one of its windows is open; records without rows are untouched. The filter is in getSakeRecordsQuery.

const ffcPlazaEventsTable = `
CREATE TABLE IF NOT EXISTS public.ffc_plaza_events (
	id serial PRIMARY KEY,
	game_id integer NOT NULL,
	table_id character varying NOT NULL,
	record_id integer NOT NULL,
	name character varying NOT NULL DEFAULT '' CHECK (LENGTH(name) <= 64),
	start_at timestamp without time zone NOT NULL,
	end_at timestamp without time zone NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
	CHECK (end_at > start_at),
	FOREIGN KEY (game_id, table_id, record_id) REFERENCES public.sake_records (game_id, table_id, record_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ffc_plaza_events_record_idx ON public.ffc_plaza_events (game_id, table_id, record_id);`

// sakeEventVisible is ANDed into getSakeRecordsQuery (alias sake_records).
const sakeEventVisible = `
		  AND (NOT EXISTS (SELECT 1 FROM ffc_plaza_events e WHERE e.game_id = sake_records.game_id AND e.table_id = sake_records.table_id AND e.record_id = sake_records.record_id)
		       OR EXISTS (SELECT 1 FROM ffc_plaza_events e WHERE e.game_id = sake_records.game_id AND e.table_id = sake_records.table_id AND e.record_id = sake_records.record_id
		                  AND (now() AT TIME ZONE 'UTC') >= e.start_at AND (now() AT TIME ZONE 'UTC') < e.end_at))`

var ErrPlazaEventInvalid = errors.New("invalid plaza event")

type PlazaEvent struct {
	ID       int64     `json:"id"`
	RecordID int32     `json:"record_id"`
	Name     string    `json:"name"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Active   bool      `json:"active"`
	OwnerID  int32     `json:"owner_id"`
	TeamName string    `json:"team_name"`
}

func (c *Connection) AddPlazaEvent(gameId int, tableId string, recordId int32, name string, start, end time.Time) (int64, error) {
	if !end.After(start) || len(name) > 64 {
		return 0, ErrPlazaEventInvalid
	}
	var id int64
	err := c.pool.QueryRow(c.ctx, `INSERT INTO ffc_plaza_events (game_id, table_id, record_id, name, start_at, end_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, gameId, tableId, recordId, name, start.UTC(), end.UTC()).Scan(&id)
	return id, err
}

func (c *Connection) DeletePlazaEvent(id int64) (bool, error) {
	tag, err := c.pool.Exec(c.ctx, `DELETE FROM ffc_plaza_events WHERE id = $1`, id)
	return tag.RowsAffected() > 0, err
}

// ListPlazaEvents: windows ending after `since` (zero = all), soonest start first, with the record's owner and
// team_name (the admin's view; ignores the visibility filter).
func (c *Connection) ListPlazaEvents(gameId int, tableId string, since time.Time) ([]PlazaEvent, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT e.id, e.record_id, e.name, e.start_at, e.end_at,
		(now() AT TIME ZONE 'UTC') >= e.start_at AND (now() AT TIME ZONE 'UTC') < e.end_at, r.owner_id,
		COALESCE(r.fields->'team_name'->>'value', '')
		FROM ffc_plaza_events e JOIN sake_records r USING (game_id, table_id, record_id)
		WHERE e.game_id = $1 AND e.table_id = $2 AND e.end_at > $3 ORDER BY e.start_at, e.id`, gameId, tableId, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlazaEvent{}
	for rows.Next() {
		var e PlazaEvent
		if err := rows.Scan(&e.ID, &e.RecordID, &e.Name, &e.Start, &e.End, &e.Active, &e.OwnerID, &e.TeamName); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
