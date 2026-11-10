package api

import (
	"net/http"
	"os"
	"strconv"
	"time"
	"wwfc/ffcstats"
	"wwfc/logging"
)

// Banned-vivosaur lists (ffc-decomp ROADMAP #75 server part, ffcstats.BanRule): banned.json next to the server,
// re-read with the match cache. Every public stat (ffcLoadMatchesCached callers) skips violating matches;
//
//	GET /api/ffc/admin/banned[?days=N]  -> {"rules": [...], "violations": [{match, rule, pids, creatures}]}
//
// (last N days, default 30, max 400). GET /api/ffc/banned (public) -> {"rules": [...]} for web/Discord.
var ffcBannedFile = "banned.json"

func ffcBanRules() []ffcstats.BanRule {
	f, err := os.Open(ffcBannedFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	rs, err := ffcstats.ParseBanRules(f)
	if err != nil {
		logging.Error("API", "FFC banned.json:", err)
		return nil
	}
	return rs
}

func HandleFFCBannedPublic(w http.ResponseWriter, r *http.Request) {
	if _, err := parseGet(r, w, RoleNone); err != nil {
		return
	}
	rs := ffcBanRules()
	if rs == nil {
		rs = []ffcstats.BanRule{}
	}
	replyOK(w, map[string]any{"rules": rs})
}

func HandleFFCBanned(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleModerator)
	if err != nil {
		return
	}
	days := 30
	if s := q.Get("days"); s != "" {
		if days, err = strconv.Atoi(s); err != nil || days < 1 || days > 400 {
			replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
			return
		}
	}
	now := ffcNow()
	ms, err := db.LoadMatches(now.Add(-time.Duration(days)*24*time.Hour), time.Time{}, ffcMaxRows)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	rs := ffcBanRules()
	if rs == nil {
		rs = []ffcstats.BanRule{}
	}
	replyOK(w, map[string]any{"rules": rs, "violations": ffcstats.Violations(ms, rs)})
}
