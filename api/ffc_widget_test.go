package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFFCWidget(t *testing.T) {
	rec := httptest.NewRecorder()
	HandleFFCWidget(rec, httptest.NewRequest("GET", "/ffc/widget?top=5", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatal(rec.Code, rec.Header())
	}
	for _, want := range []string{"var TOP=5;", `"/api/ffc/online"`, `"/api/ffc/leaderboard"`, "textContent"} {
		if !strings.Contains(body, want) {
			t.Fatal("missing", want)
		}
	}
	// no external resources, no innerHTML
	for _, bad := range []string{"http://", "https://", "<script src", "<link", "innerHTML", "__TOP__"} {
		if strings.Contains(body, bad) {
			t.Fatal("has", bad)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self'") || !strings.Contains(csp, "frame-ancestors *") || rec.Header().Get("X-Frame-Options") != "" {
		t.Fatal("csp", csp)
	}
	for q, want := range map[string]string{"": "var TOP=10;", "?top=0": "var TOP=10;", "?top=999": "var TOP=10;", "?top=x": "var TOP=10;", "?top=50": "var TOP=50;"} {
		rec = httptest.NewRecorder()
		HandleFFCWidget(rec, httptest.NewRequest("GET", "/ffc/widget"+q, nil))
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatal(q, "want", want)
		}
	}
	rec = httptest.NewRecorder()
	HandleFFCWidget(rec, httptest.NewRequest("HEAD", "/ffc/widget", nil))
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Length") == "" {
		t.Fatal("head", rec.Code, rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	HandleFFCWidget(rec, httptest.NewRequest("POST", "/ffc/widget", nil))
	if rec.Code != 405 {
		t.Fatal("post", rec.Code)
	}
}
