package ffcmatch

import (
	"testing"
	"time"
)

func host(pid, rank string) map[string]string { return map[string]string{"dwc_pid": pid, "Rank": rank} }

func pids(h []map[string]string) string {
	s := ""
	for _, x := range h {
		s += x["dwc_pid"] + ","
	}
	return s
}

func TestSearcherPid(t *testing.T) {
	f := "dwc_mver = 90 and dwc_pid != 43 and maxplayers = 2 and numplayers < 2 and dwc_mtype = 0 and dwc_hoststate = 2 and dwc_suspend = 0 and (1)"
	if p, ok := SearcherPid(f); !ok || p != "43" {
		t.Fatal(p, ok)
	}
	if _, ok := SearcherPid("dwc_mver = 90 and dwc_pid != 43 and dwc_mtype = 2"); ok {
		t.Fatal("friend/sc match type must be untouched")
	}
	if _, ok := SearcherPid("dwc_pid = 43"); ok {
		t.Fatal("self lookup")
	}
}

func TestApplyWidens(t *testing.T) {
	now := time.Unix(1000, 0)
	Now = func() time.Time { return now }
	SetConfigForTest(Default)
	searchers, lastMatch = map[string]*searcher{}, map[string]matchNote{}
	self := host("1", "10")
	hosts := []map[string]string{host("2", "11"), host("3", "15"), host("4", "30"), {"dwc_pid": "5"}}
	codes := map[string]string{"1": "VDEE", "2": "VDEE", "3": "VDEP", "4": "VDEE", "5": "VDEE"}
	gc := func(p string) string { return codes[p] }

	// t=0: window 2 -> 2 (11) and 5 (no key = 10); 3 is rank 5 away and EU anyway.
	if got, _ := Apply("1", self, hosts, gc); pids(got) != "2,5," {
		t.Fatal("t0", pids(got))
	}
	// t=10 s: window 7 -> rank 15 ok but region still VDEE only (region_sec 20).
	now = now.Add(10 * time.Second)
	if got, _ := Apply("1", self, hosts, gc); pids(got) != "2,5," {
		t.Fatal("t10", pids(got))
	}
	// t=25 s: window 14.5, region open -> 3 joins; 30 (20 away) not yet.
	now = now.Add(15 * time.Second)
	if got, _ := Apply("1", self, hosts, gc); pids(got) != "2,3,5," {
		t.Fatal("t25", pids(got))
	}
	// t=60 s: open (client queries every few seconds; a >30 s gap would restart the search).
	for i := 0; i < 7; i++ {
		now = now.Add(5 * time.Second)
		Apply("1", self, hosts, gc)
	}
	if got, _ := Apply("1", self, hosts, gc); len(got) != 4 {
		t.Fatal("t60", pids(got))
	}
}

func TestSearchGapResets(t *testing.T) {
	now := time.Unix(5000, 0)
	Now = func() time.Time { return now }
	SetConfigForTest(Default)
	searchers, lastMatch = map[string]*searcher{}, map[string]matchNote{}
	self := host("1", "1")
	hosts := []map[string]string{host("2", "20")}
	Apply("1", self, hosts, nil)
	now = now.Add(61 * time.Second) // gap > 30 s: a new search, timer restarts
	if got, el := Apply("1", self, hosts, nil); len(got) != 0 || el != 0 {
		t.Fatal(pids(got), el)
	}
}

func TestRematchAvoid(t *testing.T) {
	now := time.Unix(9000, 0)
	Now = func() time.Time { return now }
	SetConfigForTest(Default)
	searchers, lastMatch = map[string]*searcher{}, map[string]matchNote{}
	NoteMatch("1", "2")
	self := host("1", "10")
	hosts := []map[string]string{host("2", "10"), host("3", "10")}
	if got, _ := Apply("1", self, hosts, nil); pids(got) != "3," {
		t.Fatal(pids(got))
	}
	for i := 0; i < 7; i++ {
		now = now.Add(5 * time.Second)
		Apply("1", self, hosts, nil)
	}
	if got, _ := Apply("1", self, hosts, nil); pids(got) != "2,3," {
		t.Fatal("after rematch_sec", pids(got))
	}
	// Only-candidate case still opens at open_after_sec at the latest (here at rematch_sec already).
}

func TestDisabledAndNoSelf(t *testing.T) {
	now := time.Unix(20000, 0)
	Now = func() time.Time { return now }
	c := Default
	c.Enabled = false
	SetConfigForTest(c)
	hosts := []map[string]string{host("2", "99")}
	if got, _ := Apply("1", host("1", "1"), hosts, nil); len(got) != 1 {
		t.Fatal("disabled")
	}
	SetConfigForTest(Default)
	if got, _ := Apply("7", nil, hosts, nil); len(got) != 1 {
		t.Fatal("no self -> no rank rule")
	}
}
