// Package ffcmatch: server-side matchmaking tuning for Fossil Fighters Champions "Battle with Anyone"
// (ffc-decomp ROADMAP #34; design note docs/sessions/s102-online/matchmaking_tuning.md in the decomp repo).
//
// How FFC matches: every searcher hosts a QR2 session (custom key "Rank" = the best of its 3 saved teams'
// rank values, ov007 0x021b10a8) and repeatedly asks the server browser for "dwc_mver = .. and dwc_pid != <me>
// and .. dwc_mtype = 0 ..". The client scores every returned host with 9999 - |myRank - hostRank| (ov007
// 0x021b112c) and reserves the best one. So the client already prefers the closest Rank; the only server lever
// is WHICH hosts it returns. This package removes hosts from that list for a short while:
//   - Rank window: |Rank - myRank| <= RankWindow + RankWidenPerSec * seconds searching.
//   - Region: during RegionSec, only hosts whose game code ends in the same region letter (VDEE/VDEP/VDEJ).
//   - Rematch: during RematchSec, not the opponent of my last match (if it was < RematchMemory ago).
//
// After OpenAfterSec every rule is off, so nobody waits longer than stock past that point (an empty list just makes
// the client query again a few seconds later). Friend battles (other match types) and self lookups are untouched.
// Settings: defaults below, overridable by ffc_match.json in the server working dir (re-read when it changes).
package ffcmatch

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const GameName = "dental2ds"

type Config struct {
	Enabled         bool    `json:"enabled"`
	RankWindow      int     `json:"rank_window"`        // initial allowed |Rank difference|
	RankWidenPerSec float64 `json:"rank_widen_per_sec"` // window growth per second of searching
	RegionSec       int     `json:"region_sec"`         // same-region-only time; 0 = off
	RematchSec      int     `json:"rematch_sec"`        // avoid-last-opponent time; 0 = off
	RematchMemory   int     `json:"rematch_memory_sec"` // how long a match counts as "last" (seconds)
	OpenAfterSec    int     `json:"open_after_sec"`     // every rule off after this many seconds
	DefaultRank     int     `json:"default_rank"`       // host without a Rank key (the client also uses 10)
}

var Default = Config{
	Enabled: true, RankWindow: 2, RankWidenPerSec: 0.5, RegionSec: 20, RematchSec: 30, RematchMemory: 600,
	OpenAfterSec: 60, DefaultRank: 10,
}

// ConfigPath is read relative to the server working dir (like tournaments.json, seasons.tsv).
var ConfigPath = "ffc_match.json"

var (
	mu        sync.Mutex
	searchers = map[string]*searcher{} // dwc_pid -> search state
	lastMatch = map[string]matchNote{} // dwc_pid -> last opponent
	cfg       = Default
	cfgMtime  time.Time
	cfgCheck  time.Time
	Now       = time.Now
)

type searcher struct{ first, last time.Time }
type matchNote struct {
	other string
	at    time.Time
}

// SearchGap: a searcher not seen for this long starts a new search (its timer resets).
const SearchGap = 30 * time.Second

var (
	reSearcher = regexp.MustCompile(`dwc_pid != (\d{1,10})`)
	reAnybody  = regexp.MustCompile(`dwc_mtype = 0\b`)
)

// SearcherPid returns the searching profile id when the filter is a Battle with Anyone query.
func SearcherPid(filter string) (string, bool) {
	if !reAnybody.MatchString(filter) {
		return "", false
	}
	m := reSearcher.FindStringSubmatch(filter)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// GetConfig returns the active settings (file override re-read at most every 5 s).
func GetConfig() Config {
	mu.Lock()
	defer mu.Unlock()
	return loadConfigLocked()
}

func loadConfigLocked() Config {
	now := Now()
	if now.Sub(cfgCheck) < 5*time.Second {
		return cfg
	}
	cfgCheck = now
	st, err := os.Stat(ConfigPath)
	if err != nil {
		cfg, cfgMtime = Default, time.Time{}
		return cfg
	}
	if st.ModTime().Equal(cfgMtime) {
		return cfg
	}
	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		return cfg
	}
	c := Default
	if json.Unmarshal(data, &c) != nil {
		return cfg // keep the previous settings on a bad file
	}
	cfg, cfgMtime = c, st.ModTime()
	return cfg
}

// SetConfigForTest replaces the settings (tests only).
func SetConfigForTest(c Config) {
	mu.Lock()
	cfg, cfgCheck = c, Now().Add(time.Hour)
	mu.Unlock()
}

// NoteMatch records that a and b were just matched (QR2 group join).
func NoteMatch(a, b string) {
	if a == "" || b == "" || a == b {
		return
	}
	now := Now()
	mu.Lock()
	lastMatch[a] = matchNote{b, now}
	lastMatch[b] = matchNote{a, now}
	delete(searchers, a)
	delete(searchers, b)
	if len(lastMatch) > 20000 {
		for k, v := range lastMatch {
			if now.Sub(v.at) > time.Hour {
				delete(lastMatch, k)
			}
		}
	}
	mu.Unlock()
}

func rankOf(s map[string]string, def int) int {
	if v, err := strconv.Atoi(s["Rank"]); err == nil {
		return v
	}
	return def
}

func regionOf(gameCode string) byte {
	if len(gameCode) != 4 {
		return 0
	}
	return gameCode[3]
}

// Apply filters the already-matched host list for one searcher query. self = the searcher's own QR2 session data
// (nil if unknown: no rank rule); gameCode(pid) gives a profile's login game code ("" if unknown).
// Returns the hosts to send and how long the searcher has been searching.
func Apply(searcherPid string, self map[string]string, hosts []map[string]string, gameCode func(pid string) string) ([]map[string]string, time.Duration) {
	now := Now()
	mu.Lock()
	c := loadConfigLocked()
	s := searchers[searcherPid]
	if s == nil || now.Sub(s.last) > SearchGap {
		s = &searcher{first: now}
		searchers[searcherPid] = s
		if len(searchers) > 20000 {
			for k, v := range searchers {
				if now.Sub(v.last) > SearchGap {
					delete(searchers, k)
				}
			}
		}
	}
	s.last = now
	elapsed := now.Sub(s.first)
	last, hasLast := lastMatch[searcherPid]
	mu.Unlock()

	if !c.Enabled || elapsed >= time.Duration(c.OpenAfterSec)*time.Second || len(hosts) == 0 {
		return hosts, elapsed
	}

	// The searcher's own Rank: its own QR2 session (every searcher also hosts); absent -> no rank rule.
	myRank, haveRank := 0, self != nil
	if haveRank {
		myRank = rankOf(self, c.DefaultRank)
	}
	return filterHosts(c, elapsed, searcherPid, hosts, gameCode, haveRank, myRank, hasLast && now.Sub(last.at) < time.Duration(c.RematchMemory)*time.Second, last.other), elapsed
}

func filterHosts(c Config, elapsed time.Duration, me string, hosts []map[string]string, gameCode func(string) string,
	haveRank bool, myRank int, recent bool, lastOther string) []map[string]string {
	sec := elapsed.Seconds()
	window := float64(c.RankWindow) + c.RankWidenPerSec*sec
	myRegion := byte(0)
	if gameCode != nil && sec < float64(c.RegionSec) {
		myRegion = regionOf(gameCode(me))
	}
	avoid := recent && sec < float64(c.RematchSec)
	var out []map[string]string
	for _, h := range hosts {
		pid := h["dwc_pid"]
		if haveRank {
			d := rankOf(h, c.DefaultRank) - myRank
			if d < 0 {
				d = -d
			}
			if float64(d) > window {
				continue
			}
		}
		if myRegion != 0 {
			if r := regionOf(gameCode(pid)); r != 0 && r != myRegion {
				continue
			}
		}
		if avoid && pid == lastOther {
			continue
		}
		out = append(out, h)
	}
	return out
}
