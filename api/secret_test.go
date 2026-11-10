package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// adminGET builds a GET request carrying the admin secret in the header (NET-14: no ?secret= query form).
func adminGET(target, secret string) *http.Request {
	r := httptest.NewRequest("GET", target, nil)
	if secret != "" {
		r.Header.Set(SecretHeader, secret)
	}
	return r
}

// Other tests send wrong secrets from httptest's single RemoteAddr; only TestAuthFailRateLimit uses the real cap.
func init() { authFailMax = 1 << 30 }

func resetAuthFails() {
	authFailMu.Lock()
	authFails = map[string]*authFailEntry{}
	authFailMu.Unlock()
}

// NET-33: an empty apiSecret disables the admin API instead of accepting an empty secret.
func TestEmptyAPISecretRefusesAll(t *testing.T) {
	old := apiSecret
	defer func() { apiSecret = old }()
	apiSecret = ""
	if authenticate(AuthInfo{Secret: ""}, RoleModerator) {
		t.Error("empty secret accepted while apiSecret is empty")
	}
	apiSecret = "real"
	if !authenticate(AuthInfo{Secret: "real"}, RoleModerator) || authenticate(AuthInfo{Secret: ""}, RoleModerator) {
		t.Error("secret check wrong")
	}
}

// NET-14: empty configured secret + empty request secret -> 401 over HTTP (GET header and POST body);
// the ?secret= query form no longer authenticates; the header does (GET and POST).
func TestAdminSecretHTTP(t *testing.T) {
	old := apiSecret
	defer func() { apiSecret = old; resetAuthFails() }()
	resetAuthFails()
	apiSecret = ""
	rec := httptest.NewRecorder()
	HandleFFCMotd(rec, adminGET("/api/ffc/admin/motd", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("empty/empty GET:", rec.Code)
	}
	rec = httptest.NewRecorder()
	HandleFFCMotd(rec, httptest.NewRequest("POST", "/api/ffc/admin/motd", strings.NewReader(`{"secret":"","delete":true}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("empty/empty POST:", rec.Code)
	}

	apiSecret = "s3cret"
	resetAuthFails()
	rec = httptest.NewRecorder()
	if _, err := parseGet(httptest.NewRequest("GET", "/x?secret=s3cret", nil), rec, RoleModerator); err == nil || rec.Code != 401 {
		t.Fatal("?secret= query form still accepted:", rec.Code)
	}
	rec = httptest.NewRecorder()
	if _, err := parseGet(adminGET("/x", "s3cret"), rec, RoleModerator); err != nil {
		t.Fatal("header secret refused:", err)
	}
	var p parseProbe
	r := httptest.NewRequest("POST", "/x", strings.NewReader(`{}`))
	r.Header.Set(SecretHeader, "s3cret")
	if err := parsePost(r, httptest.NewRecorder(), &p, RoleModerator); err != nil {
		t.Fatal("POST header secret refused:", err)
	}
}

type parseProbe struct {
	AuthInfo
}

// NET-14: after authFailMax failures from one IP, even the right secret gets 429 until the window ends;
// another IP is unaffected.
func TestAuthFailRateLimit(t *testing.T) {
	old, oldMax := apiSecret, authFailMax
	defer func() { apiSecret = old; authFailMax = oldMax; resetAuthFails() }()
	resetAuthFails()
	apiSecret = "s3cret"
	authFailMax = 10
	for i := 0; i < authFailMax; i++ {
		rec := httptest.NewRecorder()
		parseGet(adminGET("/x", "wrong"), rec, RoleModerator)
		if rec.Code != 401 {
			t.Fatal("failure", i, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	parseGet(adminGET("/x", "s3cret"), rec, RoleModerator)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatal("not limited after", authFailMax, "failures:", rec.Code)
	}
	other := adminGET("/x", "s3cret")
	other.RemoteAddr = "198.51.100.7:4000"
	if _, err := parseGet(other, httptest.NewRecorder(), RoleModerator); err != nil {
		t.Fatal("other IP limited too:", err)
	}
}
