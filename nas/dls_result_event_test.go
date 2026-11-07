package nas

import (
	"strings"
	"testing"
	"time"
	"wwfc/database"
	"wwfc/logging"
)

// #66/#28: one webhook event per match (on confirm), a mismatch event with the field; a single report sends none.
func TestResultEvents(t *testing.T) {
	got := make(chan string, 8)
	data := make(chan map[string]any, 8)
	logging.RegisterEventCallback([]string{"match_result", "match_mismatch"}, func(ev string, d map[string]any) {
		got <- ev
		data <- d
	})
	fake := &fakeResultDB{owners: map[uint32]uint64{1000000101: 4242, 1000000202: 777}}
	old := resultDB
	resultDB = fake
	defer func() { resultDB = old }()

	wait := func() (string, map[string]any) {
		select {
		case e := <-got:
			return e, <-data
		case <-time.After(300 * time.Millisecond):
			return "", nil
		}
	}
	if r := string(dlsResult("t", goldenFields(t))); r != "ok 1 single" {
		t.Fatal(r)
	}
	if e, _ := wait(); e != "" {
		t.Fatalf("single report sent event %q", e)
	}
	fake.status = "confirmed"
	if r := string(dlsResult("t", goldenFields(t))); r != "ok 2 confirmed" {
		t.Fatal(r)
	}
	e, d := wait()
	if e != "match_result" || d["Result"] != "BOB won" || d["Queue"] != "modded" || d["Match ID"] != int64(2) ||
		!strings.Contains(d["Players"].(string), "ALICE (1000000101) vs BOB (1000000202)") {
		t.Fatalf("confirmed: %q %v", e, d)
	}
	if _, ok := d["Mismatch"]; ok {
		t.Fatal("confirmed event carries Mismatch")
	}
	fake.status, fake.mismatch = "mismatch", "winner"
	_ = dlsResult("t", goldenFields(t))
	e, d = wait()
	if e != "match_mismatch" || d["Mismatch"] != "winner" || d["Second reporter"] != uint32(1000000101) {
		t.Fatalf("mismatch: %q %v", e, d)
	}
}

func TestMatchEventDraw(t *testing.T) {
	ev, d := matchEvent(database.StoreResult{ID: 9, Status: "confirmed"},
		database.MatchReport{PidA: 1, PidB: 2, Draw: true, NameA: "A", NameB: "B"}, 1)
	if ev != "match_result" || d["Result"] != "Draw" || d["Queue"] != "vanilla" {
		t.Fatal(ev, d)
	}
}
