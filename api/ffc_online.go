package api

import (
	"net/http"
	"strconv"
	"time"
	"wwfc/database"
	"wwfc/ffcstats"
	"wwfc/gpcm"
	"wwfc/qr2"
)

// FFC live counter + admin dashboard (ffc-decomp ROADMAP #30, #33, #72).
//   GET /api/ffc/online                    public, counts only (no ids/names): for a web/Discord widget (#39).
//   GET /api/ffc/admin/dashboard?secret=…  online players (pid, name), match activity, flagged rows.

const ffcGameName = "dental2ds"

type FFCOnline struct {
	Online      int `json:"online"`      // logged in to the WFC server (GPCM), anywhere in the game's Wi-Fi menus
	Matchmaking int `json:"matchmaking"` // QR2 sessions: searching / hosting / in a match
	InBattle    int `json:"in_battle"`   // players in a group of 2+ (a running match)
	Modded      int `json:"modded"`      // QR2 sessions reporting a non-stock dwc_mver (patched ROM, modded rules)
	Vanilla     int `json:"vanilla"`     // QR2 sessions with the stock dwc_mver 90 (stock carts + the vanilla queue)
}

func countFFCOnline(players []gpcm.OnlinePlayer, servers []map[string]string, groups []qr2.GroupInfo) FFCOnline {
	c := FFCOnline{Online: len(players)}
	for _, s := range servers {
		if s["gamename"] != ffcGameName {
			continue
		}
		c.Matchmaking++
		if v := s["dwc_mver"]; v == "" || v == "90" {
			c.Vanilla++
		} else {
			c.Modded++
		}
	}
	for _, g := range groups {
		if g.GameName == ffcGameName && len(g.Players) >= 2 {
			c.InBattle += len(g.Players)
		}
	}
	return c
}

func ffcOnlineNow() (FFCOnline, []gpcm.OnlinePlayer) {
	players := gpcm.OnlinePlayers(ffcGameName)
	return countFFCOnline(players, qr2.GetSessionServers(), qr2.GetGroups([]string{ffcGameName}, []string{}, false)), players
}

func HandleFFCOnline(w http.ResponseWriter, r *http.Request) {
	if _, err := parseGet(r, w, RoleNone); err != nil {
		return
	}
	c, _ := ffcOnlineNow()
	replyOK(w, c)
}

// HandleFFCDashboard: ?days=N (activity window, default 14, max 90).
func HandleFFCDashboard(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleModerator)
	if err != nil {
		return
	}
	days, err := strconv.Atoi(q.Get("days"))
	if err != nil || days <= 0 || days > 90 {
		days = 14
	}
	now := time.Now().UTC()
	since := now.Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	ms, err := db.LoadMatches(since, time.Time{}, ffcMaxRows)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	flagged, err := db.LoadFlaggedMatches([]string{"mismatch"}, since, 1000)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	c, players := ffcOnlineNow()
	act := ffcstats.ComputeActivity(ms, now, days)
	last24 := 0
	for _, m := range ms {
		if m.CreatedAt.After(now.Add(-24 * time.Hour)) {
			last24++
		}
	}
	replyOK(w, map[string]any{
		"online":          c,
		"players":         players,
		"days":            days,
		"activity":        act,
		"peak_hour_utc":   act.PeakHour(),
		"matches_24h":     last24,
		"mismatches":      len(flagged),
		"mismatch_counts": database.FlaggedCounts(flagged),
	})
}
