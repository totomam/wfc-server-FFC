package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"wwfc/common"
	"wwfc/database"
)

func plazaRec(id, owner int32, name string, num, sum int64, win string) database.SakeRecord {
	return database.SakeRecord{RecordId: id, OwnerId: owner, NumRatings: num, SumRatings: sum, Fields: map[string]database.SakeField{
		"team_name": {Value: name}, "battle_win": {Value: win}, "battle_lose": {Value: "1"}}}
}

func TestFFCPlazaTeams(t *testing.T) {
	recs := []database.SakeRecord{plazaRec(1, 5, "a", 2, 6, "1"), plazaRec(2, ffcGhostOwner, "boss", 4, 20, "9"),
		plazaRec(3, 6, "c", 0, 0, "3"), plazaRec(4, 7, "d", 1, 5, "0")}
	got := ffcPlazaTeams(recs, "", 0, false, 0)
	if len(got) != 4 || got[0].RecordID != 2 || got[1].RecordID != 4 || got[2].RecordID != 1 || got[3].AvgRating != 0 {
		t.Fatalf("%+v", got)
	}
	if !got[0].Ghost || got[0].AvgRating != 5 || got[0].Wins != 9 || got[0].Losses != 1 || got[0].TeamName != "boss" {
		t.Fatalf("%+v", got[0])
	}
	if g := ffcPlazaTeams(recs, "ratings", 2, false, 0); len(g) != 2 || g[0].RecordID != 2 || g[1].RecordID != 1 {
		t.Fatalf("min/ratings %+v", g)
	}
	if g := ffcPlazaTeams(recs, "wins", 0, false, 2); len(g) != 2 || g[0].RecordID != 2 || g[1].RecordID != 3 {
		t.Fatalf("wins/limit %+v", g)
	}
	if g := ffcPlazaTeams(recs, "", 0, true, 0); len(g) != 1 || g[0].RecordID != 2 {
		t.Fatalf("ghosts %+v", g)
	}
}

func ffcCall(t *testing.T, h http.HandlerFunc, method, url, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(method, url, strings.NewReader(body)))
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// Live (FFC_TEST_PG=1): event windows hide/show ghost records in GetSakeRecords (= every game read path), the admin
// events API, public /api/ffc/plaza and admin /api/ffc/admin/ratings.
func TestFFCPlazaEventsPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db = database.Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	db.UpdateTables()
	apiSecret = "s3cret"
	ids := map[string]int32{}
	for _, n := range []string{"live", "later", "plain"} {
		id, err := db.InsertSakeRecord(database.SakeRecord{GameId: ffcSakeGameID, TableId: ffcSakeTable, OwnerId: ffcGhostOwner,
			Fields: map[string]database.SakeField{"team_name": {Type: database.SakeFieldTypeUnicodeString, Value: "ev-" + n},
				"battle_win": {Type: database.SakeFieldTypeInt, Value: "0"}}})
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = id
	}
	defer func() {
		for _, id := range ids {
			_ = db.DeleteSakeRecord(ffcSakeGameID, ffcSakeTable, id, ffcGhostOwner)
		}
	}()
	now := time.Now().UTC()
	post := func(body string) (int, map[string]any) {
		return ffcCall(t, HandleFFCEvents, "POST", "/api/ffc/admin/events", body)
	}
	ev := func(rec int32, s, e time.Time) string {
		b, _ := json.Marshal(map[string]any{"secret": "s3cret", "record_id": rec, "name": "Boss week", "start": s.Format(time.RFC3339), "end": e.Format(time.RFC3339)})
		return string(b)
	}
	if c, _ := post(strings.Replace(ev(ids["live"], now.Add(-time.Hour), now.Add(time.Hour)), "s3cret", "bad", 1)); c != 401 {
		t.Fatal("no secret", c)
	}
	c, out := post(ev(ids["live"], now.Add(-time.Hour), now.Add(time.Hour)))
	if c != 200 {
		t.Fatal(c, out)
	}
	if c, _ := post(ev(ids["later"], now.Add(time.Hour), now.Add(2*time.Hour))); c != 200 {
		t.Fatal("later", c)
	}
	if c, _ := post(ev(ids["later"], now.Add(time.Hour), now.Add(-time.Hour))); c != 400 {
		t.Fatal("end<start", c)
	}
	if c, _ := post(ev(123, now, now.Add(time.Hour))); c != 404 {
		t.Fatal("unknown record", c)
	}
	visible := func() map[string]bool {
		recs, err := db.GetSakeRecords(ffcSakeGameID, []int32{ffcGhostOwner}, ffcSakeTable, nil, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		v := map[string]bool{}
		for _, r := range recs {
			v[r.Fields["team_name"].Value] = true
		}
		return v
	}
	if v := visible(); !v["ev-live"] || v["ev-later"] || !v["ev-plain"] {
		t.Fatal("visibility", v)
	}
	c, out = ffcCall(t, HandleFFCEvents, "GET", "/api/ffc/admin/events?secret=s3cret", "")
	evs, _ := out["events"].([]any)
	if c != 200 || len(evs) < 2 {
		t.Fatal(c, out)
	}
	first := evs[0].(map[string]any)
	if first["active"] != true || first["team_name"] != "ev-live" {
		t.Fatal(first)
	}
	// deleting the last window of "later" makes it a normal always-on team
	var laterID float64
	for _, e := range evs {
		if m := e.(map[string]any); m["team_name"] == "ev-later" {
			laterID = m["id"].(float64)
		}
	}
	if c, _ := post(`{"secret":"s3cret","delete":true,"id":` + jsonNum(laterID) + `}`); c != 200 {
		t.Fatal("delete", c)
	}
	if c, _ := post(`{"secret":"s3cret","delete":true,"id":` + jsonNum(laterID) + `}`); c != 404 {
		t.Fatal("delete twice", c)
	}
	if v := visible(); !v["ev-later"] {
		t.Fatal("after delete", v)
	}
	// ratings: two raters on "plain"
	for i, rt := range []byte{4, 2} {
		if _, _, err := db.RateSakeRecord(ffcSakeGameID, ffcSakeTable, ids["plain"], int32(900000301+i), rt); err != nil {
			t.Fatal(err)
		}
	}
	c, out = ffcCall(t, HandleFFCPlaza, "GET", "/api/ffc/plaza?ghosts=1&min=2", "")
	teams, _ := out["teams"].([]any)
	if c != 200 || len(teams) != 1 || teams[0].(map[string]any)["avg_rating"] != 3.0 || strings.Contains(jsonOf(out), "rater") {
		t.Fatal(c, out)
	}
	if c, _ := ffcCall(t, HandleFFCRatings, "GET", "/api/ffc/admin/ratings?record_id=1", ""); c != 401 {
		t.Fatal("ratings no secret", c)
	}
	c, out = ffcCall(t, HandleFFCRatings, "GET", "/api/ffc/admin/ratings?secret=s3cret&record_id="+jsonNum(float64(ids["plain"])), "")
	if c != 200 || out["ratings"].(float64) != 2 || out["avg_rating"].(float64) != 3 || len(out["raters"].([]any)) != 2 {
		t.Fatal(c, out)
	}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }
func jsonOf(v any) string      { b, _ := json.Marshal(v); return string(b) }
