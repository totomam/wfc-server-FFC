package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"wwfc/ffcbonus"
)

func TestFFCMotdHandler(t *testing.T) {
	ffcbonus.Dir = t.TempDir()
	apiSecret = "s3cret"
	post := func(body string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCMotd(rec, httptest.NewRequest("POST", "/api/ffc/admin/motd", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	get := func(q string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCMotd(rec, adminGET("/api/ffc/admin/motd", q))
		var out struct{ Text string }
		json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out.Text
	}
	if c, _ := post(`{"secret":"no","text":"hi"}`); c != 401 {
		t.Fatal("wrong secret:", c)
	}
	if c, b := post(`{"secret":"s3cret","text":"Welcome to FFC online!"}`); c != 200 {
		t.Fatal(c, b)
	}
	if c, s := get("s3cret"); c != 200 || s != "Welcome to FFC online!" {
		t.Fatal(c, s)
	}
	if c, _ := get(""); c != 401 {
		t.Fatal("get without secret:", c)
	}
	if c, b := post(`{"secret":"s3cret","text":"` + strings.Repeat("x", 400) + `"}`); c != 400 || !strings.Contains(b, "360") {
		t.Fatal(c, b)
	}
	if c, _ := post(`{"secret":"s3cret","delete":true}`); c != 200 {
		t.Fatal(c)
	}
	if _, s := get("s3cret"); s != "" {
		t.Fatal("after delete:", s)
	}
}
