package nas

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"wwfc/common"
	"wwfc/ffcbonus"
)

func tok(user string) string {
	u, _ := strconv.ParseUint(user, 10, 64)
	return dlsIssueToken(u)
}

// NET-10: result, rename (lbname) and mail (hub/news) requests with only a guessed userid (no token, a forged token, or
// another userid's token) are rejected; svcloc svc 9000 hands out the token as "servicetoken" (the key the ROM reads).
func TestDlsGuessedUseridRejected(t *testing.T) {
	sv := svcloc("t", map[string][]byte{"svc": []byte("9000"), "userid": []byte("4242")})
	if !dlsTokenValid(sv["servicetoken"], 4242) || sv["svchost"] != "dls1.nintendowifi.net" {
		t.Fatalf("svcloc: %v", sv)
	}
	good := sv["servicetoken"]
	forged := strings.Repeat("0", 56)
	bad := map[string]map[string][]byte{
		"none":   {"userid": []byte("4242")},
		"empty":  {"userid": []byte("4242"), "token": nil},
		"forged": {"userid": []byte("4242"), "token": []byte(forged)},
		"other":  {"userid": []byte("4242"), "token": []byte(dlsIssueToken(777))},
		"static": {"userid": []byte("4242"), "token": []byte("NDS/SVCLOC/TOKEN")},
	}
	f := goldenFields(t)
	fake := &fakeResultDB{owners: map[uint32]uint64{1000000101: 4242, 1000000202: 777}}
	oldDB, oldLb, oldNews := resultDB, lbNameDB, ffcbonus.NewsSource
	resultDB, lbNameDB = fake, &fakeLbNames{pids: []uint32{1000000101}, names: map[uint32]string{}}
	ffcbonus.NewsSource = func(string, uint64) []ffcbonus.NewsItem {
		return []ffcbonus.NewsItem{{Name: ffcbonus.MailName(1), Body: []byte("SECRET MAIL")}}
	}
	newsCache = map[string]newsEntry{}
	defer func() {
		resultDB, lbNameDB, ffcbonus.NewsSource = oldDB, oldLb, oldNews
		newsCache = map[string]newsEntry{}
	}()
	for name, b := range bad {
		r := map[string][]byte{"attr1": f["attr1"], "attr2": f["attr2"], "rhgamecd": []byte("VDEE")}
		for k, v := range b {
			r[k] = v
		}
		if got := string(dlsResult("t", r)); got != "err token" {
			t.Errorf("%s: result = %q", name, got)
		}
		r["attr1"] = []byte("Rex")
		if got := string(dlsLbName("t", r)); got != "err" {
			t.Errorf("%s: lbname = %q", name, got)
		}
		r["attr1"] = []byte("mail")
		if got := string(dlsHub("t", r)); strings.Contains(got, "SECRET") {
			t.Errorf("%s: hub mail leaked: %q", name, got)
		}
	}
	if len(fake.stored) != 0 {
		t.Fatal("a rejected report was stored")
	}
	// The real token (as parsed, or posted raw so parseAuthRequest "decoded" it) is accepted.
	for _, tk := range [][]byte{[]byte(good), mustDwcDecode(t, good)} {
		r := goldenFields(t)
		r["token"] = tk
		if got := string(dlsResult("t", r)); !strings.HasPrefix(got, "ok ") {
			t.Fatalf("valid token: %q", got)
		}
		if got := string(dlsHub("t", map[string][]byte{"userid": []byte("4242"), "token": tk, "rhgamecd": []byte("VDEE"), "attr1": []byte("mail")})); !strings.Contains(got, "SECRET") {
			t.Fatalf("valid token: hub = %q", got)
		}
	}
	// Expired.
	dlsTokenNow = func() time.Time { return time.Now().Add(dlsTokenTTL + time.Minute) }
	defer func() { dlsTokenNow = time.Now }()
	if dlsTokenValid(good, 4242) {
		t.Fatal("expired token accepted")
	}
}

func mustDwcDecode(t *testing.T, s string) []byte {
	b, err := common.Base64DwcEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
