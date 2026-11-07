package api

import (
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"time"
	"wwfc/ffcstats"
	"wwfc/logging"
)

// FFC Plaza ghost badges (ffc-decomp ROADMAP #78, s104-online; sake/ffc_badge.go applies them to team_name):
//   - "#1".."#3": the top 3 of the ranked leaderboard (current season of seasons.tsv, else all time; >= 5 confirmed
//     ranked matches), the same board as /api/ffc/leaderboard and news item 98;
//   - badges.json next to the server: {"<pid>": "CHAMP", ...} (1..5 printable ASCII chars), admin-assigned (e.g.
//     tournament winners); wins over the rank badge.
// Keyed by the main profile (#53 links), so every profile of a player shows the badge. Recomputed at most every
// ffcBadgeTTL (Plaza searches return many records).

var ffcBadgesFile = "badges.json"

const ffcBadgeTTL = 60 * time.Second

var (
	ffcBadgeMu  sync.Mutex
	ffcBadgeAt  time.Time
	ffcBadgeMap map[uint32]string
)

func ffcBadgeFile() map[uint32]string {
	b, err := os.ReadFile(ffcBadgesFile)
	if err != nil {
		return nil
	}
	var raw map[string]string
	if err := json.Unmarshal(b, &raw); err != nil {
		logging.Error("API", "FFC badges.json:", err)
		return nil
	}
	out := map[uint32]string{}
	for k, v := range raw {
		pid, err := strconv.ParseUint(k, 10, 32)
		if err != nil || !ffcBadgeValid(v) {
			continue
		}
		out[uint32(pid)] = v
	}
	return out
}

func ffcBadgeValid(s string) bool {
	if len(s) == 0 || len(s) > 5 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// ffcBadgeBuild: pure part (tests): rank badges for the top 3 of rows, then admin badges over them.
func ffcBadgeBuild(rows []ffcstats.Player, admin map[uint32]string) map[uint32]string {
	out := map[uint32]string{}
	for i, p := range rows {
		if i >= 3 {
			break
		}
		out[p.Pid] = "#" + strconv.Itoa(i+1)
	}
	for pid, b := range admin {
		out[pid] = b
	}
	return out
}

func ffcBadges() map[uint32]string {
	now := ffcNow()
	ffcBadgeMu.Lock()
	defer ffcBadgeMu.Unlock()
	if ffcBadgeMap != nil && now.Sub(ffcBadgeAt) < ffcBadgeTTL {
		return ffcBadgeMap
	}
	links := ffcPlayerLinks()
	s := ffcNewsSeason(now)
	var rows []ffcstats.Player
	if ms, err := ffcLoadMatchesCached(s.Start, s.End); err != nil {
		logging.Error("API", "FFC badges: matches:", err)
	} else {
		ms = ffcstats.FilterMode(ffcstats.ApplyLinks(ffcstats.Filter(ms, nil, s.Start, s.End, []string{"confirmed"}), links), "ranked")
		rows = ffcstats.Leaderboard(ms, ffcNewsTop)
	}
	admin := map[uint32]string{}
	for pid, b := range ffcBadgeFile() {
		admin[ffcstats.Resolve(links, pid)] = b
	}
	ffcBadgeMap, ffcBadgeAt = ffcBadgeBuild(rows, admin), now
	return ffcBadgeMap
}

// ffcBadge: sake.FFCBadge.
func ffcBadge(owner uint32) string {
	return ffcBadges()[ffcstats.Resolve(ffcPlayerLinks(), owner)]
}
