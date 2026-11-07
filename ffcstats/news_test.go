package ffcstats

import (
	"strings"
	"testing"
	"time"
)

func TestNewsTexts(t *testing.T) {
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var ms []Match
	for i := 0; i < 6; i++ {
		ms = append(ms, Match{ID: int64(i + 1), PidA: 1, PidB: 2, WinnerPid: 1, NameA: "Alice\x01LongName", NameB: "Bob", Status: "confirmed", CreatedAt: at.Add(time.Duration(i) * time.Hour)})
	}
	rows := Leaderboard(ms, 0)
	p := ProfileNews(rows, 2, "s1")
	if !strings.Contains(p, "Bob  #2") || !strings.Contains(p, "W 0  L 6  D 0") || !strings.Contains(p, "(season s1)") {
		t.Fatalf("profile %q", p)
	}
	if ProfileNews(rows, 3, "") != "" {
		t.Fatal("unknown player must give no item")
	}
	top := TopNews(Leaderboard(ms, 5), 5)
	if !strings.HasPrefix(top, "TOP PLAYERS (ranked)\n1. AliceLongN ") || !strings.Contains(top, "\n2. Bob ") {
		t.Fatalf("top %q", top)
	}
	if TopNews(nil, 5) != "" {
		t.Fatal("empty top")
	}
	c := Challenge{ID: "w45", Title: "Win 3 with Frigi", Start: at, End: at.Add(7 * 24 * time.Hour), Kind: "wins", Target: 3}
	r := EvalChallenge(c, ms, at.Add(24*time.Hour))
	if s := ChallengesNews([]ChallengeResult{r}, 1); s != "CHALLENGES\nWin 3 with Frigi\n 3/3 DONE!" {
		t.Fatalf("challenges done %q", s)
	}
	if s := ChallengesNews([]ChallengeResult{r}, 2); s != "CHALLENGES\nWin 3 with Frigi\n 0/3 ends Nov 8" {
		t.Fatalf("challenges open %q", s)
	}
	r.Active = false
	if ChallengesNews([]ChallengeResult{r}, 1) != "" {
		t.Fatal("inactive challenge listed")
	}
	long := strings.Repeat("x", 2000)
	if len(newsCut(long)) != 0x3ff {
		t.Fatal("cut")
	}
}
