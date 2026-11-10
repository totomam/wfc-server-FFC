package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"wwfc/database"
	"wwfc/logging"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/logrusorgru/aurora/v3"
)

// FFC event / boss Plaza teams (ffc-decomp ROADMAP #40), admin only:
//   GET  /api/ffc/admin/events[?all=1]   windows not yet over (all=1: every row), with active flag + team_name
//   POST /api/ffc/admin/events {"secret","record_id","name","start","end"}   schedule (RFC 3339 times, end > start)
//   POST /api/ffc/admin/events {"secret","id","delete":true}                 remove one window
// The team itself is an ordinary dental2ds/tbl_main_na sake record (decomp tools/online_ghost.py, owner 999000001;
// a "boss" = a ghost with the stats/moves you set). Once a record has any window it is shown to players only inside
// one; deleting its last window makes it a normal always-on Plaza team again.

const (
	ffcSakeGameID = 3026
	ffcSakeTable  = "tbl_main_na"
)

type ffcEventRequest struct {
	AuthInfo
	ID       int64  `json:"id"`
	Delete   bool   `json:"delete"`
	RecordID int32  `json:"record_id"`
	Name     string `json:"name"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

func HandleFFCEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodOptions {
		q, err := parseGet(r, w, RoleModerator)
		if err != nil {
			return
		}
		since := time.Now().UTC()
		if q.Get("all") == "1" {
			since = time.Unix(0, 0).UTC()
		}
		evs, err := db.ListPlazaEvents(ffcSakeGameID, ffcSakeTable, since)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		replyOK(w, map[string]any{"events": evs})
		return
	}
	var req ffcEventRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	if req.Delete {
		ok, err := db.DeletePlazaEvent(req.ID)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		if !ok {
			replyError(w, http.StatusNotFound, "event_not_found")
			return
		}
		logging.Notice("API", "Plaza event", aurora.Cyan(req.ID), "deleted")
		replyOK(w, map[string]any{"id": req.ID, "deleted": true})
		return
	}
	start, err1 := time.Parse(time.RFC3339, req.Start)
	end, err2 := time.Parse(time.RFC3339, req.End)
	name := strings.TrimSpace(req.Name)
	if err1 != nil || err2 != nil || !end.After(start) || req.RecordID == 0 || len(name) > 64 {
		replyError(w, http.StatusBadRequest, "invalid_event")
		return
	}
	id, err := db.AddPlazaEvent(ffcSakeGameID, ffcSakeTable, req.RecordID, name, start, end)
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
	case errors.Is(err, database.ErrPlazaEventInvalid):
		replyError(w, http.StatusBadRequest, "invalid_event")
		return
	case errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation:
		replyError(w, http.StatusNotFound, "record_not_found")
		return
	default:
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	logging.Notice("API", "Plaza event", aurora.Cyan(id), "record", aurora.Cyan(req.RecordID), start.UTC(), "->", end.UTC())
	replyOK(w, map[string]any{"id": id, "record_id": req.RecordID, "start": start.UTC(), "end": end.UTC()})
}
