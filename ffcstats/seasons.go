package ffcstats

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// Season is one line of seasons.tsv (#27 seasons / resets): "<id>\t<name>\t<start YYYY-MM-DD>". A season runs from
// its start (00:00 UTC) to the next season's start; the last one is open. A reset = add a line, nothing is deleted.
type Season struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end,omitempty"` // zero = current season
}

func ParseSeasons(r io.Reader) ([]Season, error) {
	var out []Season
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("seasons line %d: want 3 tab-separated fields", n)
		}
		t, err := time.Parse("2006-01-02", strings.TrimSpace(f[2]))
		if err != nil {
			return nil, fmt.Errorf("seasons line %d: %v", n, err)
		}
		if len(out) > 0 && !t.After(out[len(out)-1].Start) {
			return nil, fmt.Errorf("seasons line %d: start must be after the previous season", n)
		}
		for _, s := range out {
			if s.ID == f[0] {
				return nil, fmt.Errorf("seasons line %d: duplicate id %q", n, f[0])
			}
		}
		out = append(out, Season{ID: strings.TrimSpace(f[0]), Name: strings.TrimSpace(f[1]), Start: t})
	}
	for i := 0; i+1 < len(out); i++ {
		out[i].End = out[i+1].Start
	}
	return out, sc.Err()
}

// FindSeason returns the season with that id; "current" = the last one.
func FindSeason(ss []Season, id string) (Season, bool) {
	if id == "current" && len(ss) > 0 {
		return ss[len(ss)-1], true
	}
	for _, s := range ss {
		if s.ID == id {
			return s, true
		}
	}
	return Season{}, false
}
