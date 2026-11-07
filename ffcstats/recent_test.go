package ffcstats

import (
	"strings"
	"testing"
	"time"
)

func TestRecentNews(t *testing.T) {
	t0 := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	tm := func(ids ...uint8) []Pick {
		var p []Pick
		for i, id := range ids {
			p = append(p, Pick{Slot: len(ids) - i, Creature: id})
		}
		return p
	}
	vivo := map[int]string{102: "Aeros", 191: "Zongazonga", 1: "T-Rex"}
	ms := []Match{
		{PidA: 1, PidB: 2, WinnerPid: 1, NameA: "Me", NameB: "Old", TeamA: tm(1), TeamB: tm(1), Status: "confirmed", CreatedAt: t0},
		{PidA: 2, PidB: 1, WinnerPid: 2, NameA: "Bob", NameB: "Me", TeamA: tm(191, 102), TeamB: tm(102), Status: "confirmed", Queue: "casual", CreatedAt: t0.Add(3 * time.Hour)},
		{PidA: 1, PidB: 3, Draw: true, NameA: "Me", NameB: "Cy", TeamA: tm(102), TeamB: tm(77), Status: "confirmed", CreatedAt: t0.Add(2 * time.Hour)},
		{PidA: 1, PidB: 4, WinnerPid: 1, NameA: "Me", NameB: "Dee", TeamA: tm(102), TeamB: tm(1), Status: "confirmed", CreatedAt: t0.Add(1 * time.Hour)},
		{PidA: 1, PidB: 5, WinnerPid: 1, NameA: "Me", NameB: "Mis", Status: "mismatch", CreatedAt: t0.Add(5 * time.Hour)},
	}
	got := RecentNews(ms, 1, vivo)
	want := "YOUR LAST MATCHES\nLOSS vs Bob (casual)\n you: Aeros\n foe: Aeros,Zongazonga\n" +
		"DRAW vs Cy (ranked)\n you: Aeros\n foe: #77\nWIN vs Dee (ranked)\n you: Aeros\n foe: T-Rex"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if n := strings.Count(got, "\n") + 1; n > newsRows {
		t.Fatal("rows", n)
	}
	if RecentNews(ms, 9, vivo) != "" {
		t.Fatal("no matches -> empty")
	}
	long := []Match{{PidA: 1, PidB: 2, WinnerPid: 1, NameA: "Me", NameB: "X", TeamA: tm(191, 191, 191, 191, 191), Status: "confirmed"}}
	for _, l := range strings.Split(RecentNews(long, 1, vivo), "\n") {
		if len(l) > newsCols {
			t.Fatal("line too long:", l)
		}
	}
}
