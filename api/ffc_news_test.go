package api

import (
	"testing"
	"time"
	"wwfc/ffcstats"
)

func TestFFCNewsItems(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ms := []ffcstats.Match{{ID: 1, PidA: 1, PidB: 2, WinnerPid: 1, NameA: "A", NameB: "B", Status: "confirmed", CreatedAt: at}}
	items := ffcNewsItems(ms, "", 1, nil)
	if len(items) != 1 || items[0].Name != "97" { // top needs >= 5 matches, no challenges
		t.Fatalf("items %+v", items)
	}
	if got := ffcNewsItems(nil, "", 1, nil); len(got) != 0 {
		t.Fatalf("no matches: %+v", got)
	}
	old := ffcSeasonsFile
	defer func() { ffcSeasonsFile = old }()
	ffcSeasonsFile = t.TempDir() + "/none.tsv"
	if s := ffcNewsSeason(at); s.ID != "" || !s.Start.IsZero() {
		t.Fatalf("no seasons file -> all time, got %+v", s)
	}
}
