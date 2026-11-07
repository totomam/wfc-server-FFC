package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"wwfc/common"
	"wwfc/database"
)

func TestFFCWindowParse(t *testing.T) {
	dir := t.TempDir()
	ffcSeasonsFile = filepath.Join(dir, "seasons.tsv")
	_ = os.WriteFile(ffcSeasonsFile, []byte("s1\tLaunch\t2026-11-01\ns2\tWinter\t2026-12-01\n"), 0o644)
	for q, want := range map[string]bool{"season=s1": true, "season=current": true, "season=zz": false,
		"from=2026-11-01&to=2026-11-02": true, "from=11/01": false, "queue=modded": true, "queue=x": false,
		"status=all": true, "status=x": false, "": true} {
		r := httptest.NewRequest("GET", "/api/ffc/leaderboard?"+q, nil)
		_, ok := parseFFCWindow(r.URL.Query())
		if ok != want {
			t.Errorf("%q: ok=%v", q, ok)
		}
	}
	w, _ := parseFFCWindow(httptest.NewRequest("GET", "/?season=s1&queue=vanilla", nil).URL.Query())
	if w.season != "s1" || w.to.Format("2006-01-02") != "2026-12-01" || w.modded == nil || *w.modded {
		t.Fatalf("%+v", w)
	}
}

// Live: the HTTP handlers over a real match_results table (FFC_TEST_PG=1, run after database's live test).
func TestFFCHandlersPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db = database.Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	get := func(url string, h func(w2 *httptest.ResponseRecorder)) map[string]any {
		rec := httptest.NewRecorder()
		h(rec)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 {
			t.Fatalf("%s: %d %s", url, rec.Code, rec.Body.String())
		}
		return out
	}
	lb := get("lb", func(rec *httptest.ResponseRecorder) {
		HandleFFCLeaderboard(rec, httptest.NewRequest("GET", "/api/ffc/leaderboard", nil))
	})
	if lb["players"] == nil {
		t.Fatal(lb)
	}
	vv := get("viv", func(rec *httptest.ResponseRecorder) {
		HandleFFCVivosaurs(rec, httptest.NewRequest("GET", "/api/ffc/vivosaurs?queue=vanilla", nil))
	})
	if vv["teams"].(float64) < 2 {
		t.Fatal(vv)
	}
	h := get("h2h", func(rec *httptest.ResponseRecorder) {
		HandleFFCH2H(rec, httptest.NewRequest("GET", "/api/ffc/h2h?a=900000002&b=900000001", nil))
	})
	if h["wins_a"].(float64) < 1 {
		t.Fatal(h)
	}
	m := get("matches", func(rec *httptest.ResponseRecorder) {
		HandleFFCMatches(rec, httptest.NewRequest("GET", "/api/ffc/matches?pid=900000001&limit=5", nil))
	})
	if len(m["matches"].([]any)) < 1 {
		t.Fatal(m)
	}
	rec := httptest.NewRecorder()
	HandleFFCH2H(rec, httptest.NewRequest("GET", "/api/ffc/h2h?a=1&b=1", nil))
	if rec.Code != 400 {
		t.Fatal("same pid must be 400")
	}
	tm := get("teams", func(rec *httptest.ResponseRecorder) {
		HandleFFCTeams(rec, httptest.NewRequest("GET", "/api/ffc/teams?status=all", nil))
	})
	if tm["teams_total"].(float64) < 2 || len(tm["teams"].([]any)) < 1 {
		t.Fatal(tm)
	}
	// #28 admin list: needs the API secret; lists mismatch rows (database live test leaves one).
	apiSecret = "s3cret"
	rec = httptest.NewRecorder()
	HandleFFCFlagged(rec, httptest.NewRequest("GET", "/api/ffc/admin/flagged", nil))
	if rec.Code != 401 {
		t.Fatal("flagged without secret must be 401, got", rec.Code)
	}
	fl := get("flagged", func(rec *httptest.ResponseRecorder) {
		HandleFFCFlagged(rec, httptest.NewRequest("GET", "/api/ffc/admin/flagged?secret=s3cret&status=both&days=1", nil))
	})
	if len(fl["rows"].([]any)) < 1 || fl["profiles"] == nil {
		t.Fatal(fl)
	}
	rec = httptest.NewRecorder()
	HandleFFCFlagged(rec, httptest.NewRequest("GET", "/api/ffc/admin/flagged?secret=s3cret&status=bogus", nil))
	if rec.Code != 400 {
		t.Fatal("bad status must be 400")
	}
}

func TestFFCFlaggedStatuses(t *testing.T) {
	for in, want := range map[string]int{"": 1, "mismatch": 1, "single": 1, "both": 2} {
		if s, ok := ffcFlaggedStatuses(in); !ok || len(s) != want {
			t.Fatal(in, s, ok)
		}
	}
	if _, ok := ffcFlaggedStatuses("confirmed"); ok {
		t.Fatal("confirmed is not an admin filter")
	}
}
