package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"wwfc/ffcbonus"
)

func TestFFCMailHandler(t *testing.T) {
	ffcbonus.Dir = t.TempDir()
	apiSecret = "s3cret"
	ffcLinksMu.Lock()
	ffcLinks, ffcLinksAt = map[uint32]uint32{20: 10}, ffcNow() // profile 20 is an alias of 10
	ffcLinksMu.Unlock()
	defer ffcLinksChanged()
	post := func(body string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCMail(rec, httptest.NewRequest("POST", "/api/ffc/admin/mail", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if c, _ := post(`{"secret":"no","pid":10,"text":"hi"}`); c != 401 {
		t.Fatal("wrong secret:", c)
	}
	if c, b := post(`{"secret":"s3cret","pid":20,"text":"Welcome!"}`); c != 200 || !strings.Contains(b, `"pid":10`) || !strings.Contains(b, `"slot":1`) {
		t.Fatal(c, b)
	}
	if c, b := post(`{"secret":"s3cret","pid":10,"slot":40,"text":"x"}`); c != 400 {
		t.Fatal(c, b)
	}
	if c, b := post(`{"secret":"s3cret","text":"x"}`); c != 400 {
		t.Fatal("no pid:", c, b)
	}
	rec := httptest.NewRecorder()
	HandleFFCMail(rec, adminGET("/api/ffc/admin/mail?pid=10", "s3cret"))
	var out struct {
		Pid  uint32
		Mail []ffcbonus.Mail
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Pid != 10 || len(out.Mail) != 1 || out.Mail[0].Text != "Welcome!" {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if it := ffcbonus.MailItems(ffcBonusGame, 10); len(it) != 1 || it[0].Name != "165" {
		t.Fatalf("items %+v", it)
	}
	if c, b := post(`{"secret":"s3cret","pid":10,"slot":1,"delete":true}`); c != 200 || !strings.Contains(b, "deleted") {
		t.Fatal(c, b)
	}
	post(`{"secret":"s3cret","pid":10,"text":"a"}`)
	if c, b := post(`{"secret":"s3cret","pid":20,"clear":true}`); c != 200 || ffcbonus.MailItems(ffcBonusGame, 10) != nil {
		t.Fatal("clear:", c, b)
	}
}
