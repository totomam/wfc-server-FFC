package nas

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dated package overrides (ffc-decomp ROADMAP #82 holiday / event reskins, s106-online). package/events.tsv lists
// windows "<event>\t<start>\t<end>" (RFC 3339 or YYYY-MM-DD = 00:00 UTC; end exclusive; '#' comments). While a
// window is open and package/<game>.<event>.bin exists, the "package" action serves that file instead of
// <game>.bin; balance/<game>.<event>.bin likewise overrides the table when present. First open window wins.
// Every event package is a full signed package (hack code + reskin) and must carry a HIGHER counter than the base
// package, and the base package re-signed with a higher counter again after the event: the loader refuses a lower
// counter until the game restarts (anti-replay floor per boot, net_pkg.c).
var eventsFile = "events.tsv"

type pkgEvent struct {
	Name       string
	Start, End time.Time
}

func parseEventTime(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// validEventName keeps the name a plain file-name part (no path, no dot).
func validEventName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func readPkgEvents(dir string) []pkgEvent {
	f, err := os.Open(filepath.Join(dir, eventsFile))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []pkgEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, "\t")
		if len(p) < 3 || !validEventName(strings.TrimSpace(p[0])) {
			continue
		}
		s, ok1 := parseEventTime(strings.TrimSpace(p[1]))
		e, ok2 := parseEventTime(strings.TrimSpace(p[2]))
		if !ok1 || !ok2 || !e.After(s) {
			continue
		}
		out = append(out, pkgEvent{strings.TrimSpace(p[0]), s, e})
	}
	return out
}

// activeEventFile returns <dir>/<game>.<event>.bin for the first open window whose file exists, else
// <dir>/<game>.bin. events.tsv is always read from packageDir (one calendar for package and table).
func activeEventFile(dir, game string, now time.Time) string {
	for _, ev := range readPkgEvents(packageDir) {
		if now.Before(ev.Start) || !now.Before(ev.End) {
			continue
		}
		p := filepath.Join(dir, game+"."+ev.Name+".bin")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dir, game+".bin")
}
