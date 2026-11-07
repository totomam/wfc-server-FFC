package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"wwfc/common"
	"wwfc/database"
	"wwfc/gpcm"
	"wwfc/qr2"
)

func TestCountFFCOnline(t *testing.T) {
	players := []gpcm.OnlinePlayer{{ProfileID: 1}, {ProfileID: 2}, {ProfileID: 3}}
	servers := []map[string]string{
		{"gamename": "dental2ds", "dwc_mver": "90"},
		{"gamename": "dental2ds", "dwc_mver": "1073746484"},
		{"gamename": "dental2ds"},
		{"gamename": "mariokartwii", "dwc_mver": "90"},
	}
	groups := []qr2.GroupInfo{
		{GameName: "dental2ds", Players: map[string]qr2.PlayerInfo{"0": {}, "1": {}}},
		{GameName: "dental2ds", Players: map[string]qr2.PlayerInfo{"0": {}}},
		{GameName: "mariokartwii", Players: map[string]qr2.PlayerInfo{"0": {}, "1": {}}},
	}
	c := countFFCOnline(players, servers, groups)
	if c != (FFCOnline{Online: 3, Matchmaking: 3, InBattle: 2, Modded: 1, Vanilla: 2}) {
		t.Fatalf("%+v", c)
	}
}

// Live (FFC_TEST_PG=1, after the database live tests): dashboard needs the secret, returns activity over real rows;
// the public counter answers without one.
func TestFFCDashboardPostgres(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db = database.Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	apiSecret = "s3cret"
	rec := httptest.NewRecorder()
	HandleFFCDashboard(rec, httptest.NewRequest("GET", "/api/ffc/admin/dashboard?days=2", nil))
	if rec.Code != 401 {
		t.Fatal("dashboard without secret:", rec.Code)
	}
	rec = httptest.NewRecorder()
	HandleFFCDashboard(rec, httptest.NewRequest("GET", "/api/ffc/admin/dashboard?days=2&secret=s3cret", nil))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	act := out["activity"].(map[string]any)
	if act["total"].(float64) < 1 || len(act["by_day"].([]any)) != 2 || out["mismatches"].(float64) < 1 {
		t.Fatal(out)
	}
	rec = httptest.NewRecorder()
	HandleFFCOnline(rec, httptest.NewRequest("GET", "/api/ffc/online", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"online":0`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
}
