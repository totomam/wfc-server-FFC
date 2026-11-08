package api

import (
	"net/http"
	"sync"
	"time"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC leaderboard names typed in game (ffc-decomp ROADMAP #52, s113-online; Nd action "lbname", nas/dls_lbname.go).
// Every /api/ffc stats view + the widget shows them (ffcstats.Names, applied by ApplyLinks). Admin moderation:
//   GET  /api/ffc/admin/names?secret=…                    every profile -> name
//   POST /api/ffc/admin/names {"secret","pid"}            delete one name (the game asks that player again)

type ffcNameRequest struct {
	AuthInfo
	Pid uint32 `json:"pid"`
}

var (
	ffcNamesMu sync.Mutex
	ffcNamesAt time.Time
	ffcNames   map[uint32]string
)

// ffcLbNames: the name map, reused for ffcCacheTTL (the last good map on a database error).
func ffcLbNames() map[uint32]string {
	now := ffcNow()
	ffcNamesMu.Lock()
	defer ffcNamesMu.Unlock()
	if ffcNames != nil && now.Sub(ffcNamesAt) < ffcCacheTTL {
		return ffcNames
	}
	m, err := db.LbNames()
	if err != nil {
		logging.Error("API", "FFC leaderboard names:", err)
		return ffcNames
	}
	ffcNames, ffcNamesAt = m, now
	return m
}

func HandleFFCNames(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodOptions {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		m, err := db.LbNames()
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		replyOK(w, map[string]any{"names": m})
		return
	}
	var req ffcNameRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	ok, err := db.DeleteLbName(req.Pid)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	if !ok {
		replyError(w, http.StatusNotFound, "name_not_found")
		return
	}
	ffcNamesMu.Lock()
	ffcNames = nil
	ffcNamesMu.Unlock()
	logging.Notice("API", "FFC leaderboard name of", aurora.Cyan(req.Pid), "removed")
	replyOK(w, map[string]any{"pid": req.Pid, "deleted": true})
}
