package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"wwfc/ffcbonus"
)

func TestFFCBonusHandler(t *testing.T) {
	ffcbonus.Dir = t.TempDir()
	apiSecret = "s3cret"
	post := func(body string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCBonus(rec, httptest.NewRequest("POST", "/api/ffc/admin/bonus", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if c, _ := post(`{"secret":"no","id":9,"text":"hi"}`); c != 401 {
		t.Fatal("wrong secret:", c)
	}
	if c, b := post(`{"secret":"s3cret","id":9,"text":"Season 2!"}`); c != 200 {
		t.Fatal(c, b)
	}
	if c, b := post(`{"secret":"s3cret","id":99,"text":"x"}`); c != 400 || !strings.Contains(b, "65..96") {
		t.Fatal(c, b)
	}
	rec := httptest.NewRecorder()
	HandleFFCBonus(rec, httptest.NewRequest("GET", "/api/ffc/admin/bonus?secret=s3cret", nil))
	var out struct{ Messages []ffcbonus.Message }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Messages) != 1 || out.Messages[0].Text != "Season 2!" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	HandleFFCBonus(rec, httptest.NewRequest("GET", "/api/ffc/admin/bonus", nil))
	if rec.Code != 401 {
		t.Fatal("list without secret:", rec.Code)
	}
	if c, _ := post(`{"secret":"s3cret","id":9,"delete":true}`); c != 200 {
		t.Fatal(c)
	}
	if ms, _ := ffcbonus.List("VDEE"); len(ms) != 0 {
		t.Fatal(ms)
	}
}
