package api

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
	"wwfc/database"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC persistent player identity (ffc-decomp ROADMAP #53), admin only:
//   GET  /api/ffc/admin/links                                  every alias -> canonical link
//   POST /api/ffc/admin/links {"secret","alias","canonical","note"}     link (alias's matches count as canonical's)
//   POST /api/ffc/admin/links {"secret","alias","delete":true}          unlink
// All public /api/ffc/* stats apply the links (pid query parameters too); match_results rows are not rewritten.

type ffcLinkRequest struct {
	AuthInfo
	Alias     uint32 `json:"alias"`
	Canonical uint32 `json:"canonical"`
	Note      string `json:"note"`
	Delete    bool   `json:"delete"`
}

var (
	ffcLinksMu sync.Mutex
	ffcLinksAt time.Time
	ffcLinks   map[uint32]uint32
)

// ffcPlayerLinks: the alias map, reused for ffcCacheTTL (nil on a database error = no linking).
func ffcPlayerLinks() map[uint32]uint32 {
	now := ffcNow()
	ffcLinksMu.Lock()
	defer ffcLinksMu.Unlock()
	if ffcLinks != nil && now.Sub(ffcLinksAt) < ffcCacheTTL {
		return ffcLinks
	}
	m, err := db.PlayerLinks()
	if err != nil {
		logging.Error("API", "FFC player links:", err)
		return ffcLinks
	}
	ffcLinks, ffcLinksAt = m, now
	return m
}

func ffcLinksChanged() {
	ffcLinksMu.Lock()
	ffcLinks = nil
	ffcLinksMu.Unlock()
}

func HandleFFCLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodOptions {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		ls, err := db.ListPlayerLinks()
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		replyOK(w, map[string]any{"links": ls})
		return
	}
	var req ffcLinkRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	if req.Delete {
		ok, err := db.DeletePlayerLink(req.Alias)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		if !ok {
			replyError(w, http.StatusNotFound, "link_not_found")
			return
		}
		ffcLinksChanged()
		logging.Notice("API", "FFC player link", aurora.Cyan(req.Alias), "removed")
		replyOK(w, map[string]any{"alias": req.Alias, "deleted": true})
		return
	}
	canon, err := db.AddPlayerLink(req.Alias, req.Canonical, strings.TrimSpace(req.Note))
	switch {
	case err == nil:
	case errors.Is(err, database.ErrPlayerLinkInvalid):
		replyError(w, http.StatusBadRequest, "invalid_link")
		return
	case errors.Is(err, database.ErrPlayerLinkNoProfile):
		replyError(w, http.StatusNotFound, "profile_not_found")
		return
	default:
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	ffcLinksChanged()
	logging.Notice("API", "FFC player link", aurora.Cyan(req.Alias), "->", aurora.Cyan(canon))
	replyOK(w, map[string]any{"alias": req.Alias, "canonical": canon})
}
