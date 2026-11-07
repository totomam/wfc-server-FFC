package api

import (
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"wwfc/database"
	"wwfc/ffcstats"
)

// Public, read-only FFC stats API (ffc-decomp ROADMAP #67; feeds the web/Discord board #39/#45). No auth: it only
// exposes what a public leaderboard shows (profile ids, in-game names, records, teams). Common query parameters:
//   season=<id>|current (seasons.tsv next to the server, #27) or from/to=YYYY-MM-DD (UTC, to exclusive);
//   queue=modded|vanilla (default both); status=confirmed (default) | all (adds single + mismatch rows).
// Endpoints: /api/ffc/leaderboard (?min=N played), /api/ffc/vivosaurs, /api/ffc/h2h?a=PID&b=PID,
// /api/ffc/matches (?pid=PID&limit=N, newest first, #32), /api/ffc/seasons.

const ffcMaxRows = 200000

var ffcSeasonsFile = "seasons.tsv"

func ffcSeasons() []ffcstats.Season {
	f, err := os.Open(ffcSeasonsFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	ss, err := ffcstats.ParseSeasons(f)
	if err != nil {
		return nil
	}
	return ss
}

type ffcWindow struct {
	from, to time.Time
	modded   *bool
	statuses []string
	season   string
	mode     string // #35 ranked (default) | casual | all
}

func parseFFCWindow(q url.Values) (ffcWindow, bool) {
	var w ffcWindow
	if s := q.Get("season"); s != "" {
		se, ok := ffcstats.FindSeason(ffcSeasons(), s)
		if !ok {
			return w, false
		}
		w.from, w.to, w.season = se.Start, se.End, se.ID
	}
	for _, p := range []struct {
		key string
		dst *time.Time
	}{{"from", &w.from}, {"to", &w.to}} {
		if v := q.Get(p.key); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return w, false
			}
			*p.dst = t
		}
	}
	switch q.Get("queue") {
	case "":
	case "modded":
		v := true
		w.modded = &v
	case "vanilla":
		v := false
		w.modded = &v
	default:
		return w, false
	}
	switch w.mode = q.Get("mode"); w.mode {
	case "", "ranked", "casual", "all":
	default:
		return w, false
	}
	switch q.Get("status") {
	case "", "confirmed":
	case "all":
		w.statuses = []string{"confirmed", "single", "mismatch"}
	default:
		return w, false
	}
	return w, true
}

func ffcLoad(w http.ResponseWriter, r *http.Request) (url.Values, []ffcstats.Match, ffcWindow, bool) {
	q, err := parseGet(r, w, RoleNone)
	if err != nil {
		return nil, nil, ffcWindow{}, false
	}
	win, ok := parseFFCWindow(q)
	if !ok {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return nil, nil, win, false
	}
	ms, err := ffcLoadMatchesCached(win.from, win.to)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return nil, nil, win, false
	}
	ms = ffcstats.ApplyLinks(ffcstats.Filter(ms, win.modded, win.from, win.to, win.statuses), ffcPlayerLinks())
	return q, ffcstats.FilterMode(ms, win.mode), win, true
}

// qPid: a profile id query parameter, resolved to its canonical profile (#53 links).
func qPid(q url.Values, key string) (uint32, bool) {
	v, err := strconv.ParseUint(q.Get(key), 10, 32)
	return ffcstats.Resolve(ffcPlayerLinks(), uint32(v)), err == nil && v != 0
}

func HandleFFCLeaderboard(w http.ResponseWriter, r *http.Request) {
	q, ms, win, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	min, _ := strconv.Atoi(q.Get("min"))
	replyOK(w, map[string]any{"season": win.season, "matches": len(ms), "players": ffcstats.Leaderboard(ms, min)})
}

func HandleFFCVivosaurs(w http.ResponseWriter, r *http.Request) {
	_, ms, win, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	rows, teams := ffcstats.Vivosaurs(ms)
	replyOK(w, map[string]any{"season": win.season, "teams": teams, "vivosaurs": rows})
}

func HandleFFCH2H(w http.ResponseWriter, r *http.Request) {
	q, ms, _, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	a, okA := qPid(q, "a")
	b, okB := qPid(q, "b")
	if !okA || !okB || a == b {
		replyError(w, http.StatusBadRequest, APIErrorInvalidProfileID)
		return
	}
	replyOK(w, ffcstats.H2H(ms, a, b))
}

func HandleFFCMatches(w http.ResponseWriter, r *http.Request) {
	q, ms, _, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	pid, hasPid := qPid(q, "pid")
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 50
	}
	out := []ffcstats.Match{}
	for i := len(ms) - 1; i >= 0 && len(out) < limit; i-- {
		if !hasPid || ms[i].PidA == pid || ms[i].PidB == pid {
			out = append(out, ms[i])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	replyOK(w, map[string]any{"matches": out})
}

func HandleFFCSeasons(w http.ResponseWriter, r *http.Request) {
	if _, err := parseGet(r, w, RoleNone); err != nil {
		return
	}
	ss := ffcSeasons()
	if ss == nil {
		ss = []ffcstats.Season{}
	}
	replyOK(w, map[string]any{"seasons": ss})
}

// HandleFFCFlagged (admin, ffc-decomp ROADMAP #28): match_results rows that need a moderator, newest first, plus the
// mismatch count per profile. GET /api/ffc/admin/flagged?secret=<apiSecret>&status=mismatch (default) | single |
// both &days=N (default 30) &limit=N (default 100, max 1000). Follow up with /api/ban (reason required, #37).
func HandleFFCFlagged(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleModerator)
	if err != nil {
		return
	}
	statuses, ok := ffcFlaggedStatuses(q.Get("status"))
	if !ok {
		replyError(w, http.StatusBadRequest, APIErrorInvalidQuery)
		return
	}
	days, err := strconv.Atoi(q.Get("days"))
	if err != nil || days <= 0 || days > 3650 {
		days = 30
	}
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := db.LoadFlaggedMatches(statuses, time.Now().UTC().AddDate(0, 0, -days), limit)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	counts := []map[string]any{}
	for pid, n := range database.FlaggedCounts(rows) {
		counts = append(counts, map[string]any{"pid": pid, "mismatches": n})
	}
	sort.Slice(counts, func(i, j int) bool {
		ni, nj := counts[i]["mismatches"].(int), counts[j]["mismatches"].(int)
		if ni != nj {
			return ni > nj
		}
		return counts[i]["pid"].(uint32) < counts[j]["pid"].(uint32)
	})
	replyOK(w, map[string]any{"rows": rows, "profiles": counts})
}

func ffcFlaggedStatuses(s string) ([]string, bool) {
	switch s {
	case "", "mismatch":
		return []string{"mismatch"}, true
	case "single":
		return []string{"single"}, true
	case "both":
		return []string{"mismatch", "single"}, true
	}
	return nil, false
}

// HandleFFCTeams (#24): team compositions + creature pairs, ?min=N picks (default 1), ?limit=N rows each (default 50).
func HandleFFCTeams(w http.ResponseWriter, r *http.Request) {
	q, ms, win, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	min, err := strconv.Atoi(q.Get("min"))
	if err != nil || min < 1 {
		min = 1
	}
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 50
	}
	teams, pairs, total := ffcstats.Teams(ms, min)
	if len(teams) > limit {
		teams = teams[:limit]
	}
	if len(pairs) > limit {
		pairs = pairs[:limit]
	}
	replyOK(w, map[string]any{"season": win.season, "teams_total": total, "teams": teams, "pairs": pairs})
}

// HandleFFCBalance (#29): matches grouped by rules version (dwc_mver), vivosaur rates per version, ?top=N (default 20).
func HandleFFCBalance(w http.ResponseWriter, r *http.Request) {
	q, ms, win, ok := ffcLoad(w, r)
	if !ok {
		return
	}
	top, err := strconv.Atoi(q.Get("top"))
	if err != nil || top < 0 || top > 210 {
		top = 20
	}
	replyOK(w, map[string]any{"season": win.season, "versions": ffcstats.BalanceImpact(ms, top)})
}

// Tournaments (ffc-decomp ROADMAP #45 server part): tournaments.json next to the server (ffcstats.Tournament).
// GET /api/ffc/tournaments -> every tournament with its bracket resolved from confirmed match_results;
// ?id=X -> one. Players are canonical profiles (#53 links). The Discord bot / web page reads this.
var ffcTournamentsFile = "tournaments.json"

func ffcTournaments() []ffcstats.Tournament {
	f, err := os.Open(ffcTournamentsFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	ts, err := ffcstats.ParseTournaments(f)
	if err != nil {
		return nil
	}
	return ts
}

func ffcBrackets(ts []ffcstats.Tournament) ([]ffcstats.Bracket, error) {
	out := []ffcstats.Bracket{}
	links := ffcPlayerLinks()
	for _, t := range ts {
		to := t.End
		if to.IsZero() {
			to = time.Now().UTC().Add(time.Hour)
		}
		ms, err := ffcLoadMatchesCached(t.Start, to)
		if err != nil {
			return nil, err
		}
		ms = ffcstats.ApplyLinks(ffcstats.Filter(ms, nil, t.Start, to, []string{"confirmed"}), links)
		for i, p := range t.Players {
			t.Players[i] = ffcstats.Resolve(links, p)
		}
		out = append(out, ffcstats.ResolveBracket(t, ms))
	}
	return out, nil
}

func HandleFFCTournaments(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleNone)
	if err != nil {
		return
	}
	ts := ffcTournaments()
	if id := q.Get("id"); id != "" {
		t, ok := ffcstats.FindTournament(ts, id)
		if !ok {
			replyError(w, http.StatusNotFound, "tournament_not_found")
			return
		}
		ts = []ffcstats.Tournament{t}
	}
	bs, err := ffcBrackets(ts)
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	replyOK(w, map[string]any{"tournaments": bs})
}

// Rewards feed (ffc-decomp ROADMAP #43 server part): who earned a reward, for a Discord bot to assign roles.
// GET /api/ffc/rewards -> champions of finished tournaments + the top 3 (Elo, >= 5 confirmed ranked games) of every
// finished season (seasons.tsv). No Discord token lives on the server; the bot polls this.
type ffcReward struct {
	Pid    uint32 `json:"pid"`
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind"` // tournament_champion | season_top3
	Source string `json:"source"`
	Place  int    `json:"place"`
}

func HandleFFCRewards(w http.ResponseWriter, r *http.Request) {
	if _, err := parseGet(r, w, RoleNone); err != nil {
		return
	}
	rewards := []ffcReward{}
	bs, err := ffcBrackets(ffcTournaments())
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	for _, b := range bs {
		if b.Finished {
			rewards = append(rewards, ffcReward{Pid: b.Champion, Kind: "tournament_champion", Source: b.ID, Place: 1})
		}
	}
	now := time.Now().UTC()
	for _, s := range ffcSeasons() {
		if s.End.IsZero() || s.End.After(now) {
			continue
		}
		ms, err := ffcLoadMatchesCached(s.Start, s.End)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "database_error")
			return
		}
		ms = ffcstats.FilterMode(ffcstats.ApplyLinks(ffcstats.Filter(ms, nil, s.Start, s.End, []string{"confirmed"}), ffcPlayerLinks()), "ranked")
		for i, p := range ffcstats.Leaderboard(ms, 5) {
			if i >= 3 {
				break
			}
			rewards = append(rewards, ffcReward{Pid: p.Pid, Name: p.Name, Kind: "season_top3", Source: s.ID, Place: i + 1})
		}
	}
	replyOK(w, map[string]any{"rewards": rewards})
}
