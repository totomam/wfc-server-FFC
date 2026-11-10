package nas

import (
	"strings"
	"testing"
	"time"
	"wwfc/ffcbonus"
)

// #76: generated news comes first in count/list/contents, keeps the same text for newsTTL (list size == contents
// length), is per console, hides a disk file of the same name, and drops bad items.
func TestNewsServed(t *testing.T) {
	old, oldSrc, oldNow := dlcDir, ffcbonus.NewsSource, newsNow
	dlcDir = t.TempDir()
	ffcbonus.Dir = dlcDir
	newsCache = map[string]newsEntry{}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	newsNow = func() time.Time { return now }
	defer func() { dlcDir, ffcbonus.NewsSource, newsNow = old, oldSrc, oldNow }()
	_ = ffcbonus.Set("VDEE", 12, "msg")
	_ = ffcbonus.Set("VDEE", 70, "admin news")
	_ = ffcbonus.Set("VDEE", 12, "msg")
	calls := 0
	text := "Elo 1000"
	ffcbonus.NewsSource = func(gamecd string, user uint64) []ffcbonus.NewsItem {
		calls++
		if user != 42 {
			return nil
		}
		return []ffcbonus.NewsItem{{Name: "97", Body: []byte(text)}, {Name: "5", Body: []byte("bad id")}, {Name: "98", Body: nil}}
	}
	f := map[string][]byte{"rhgamecd": []byte("VDEE"), "userid": []byte("42"), "token": []byte(dlsIssueToken(42))}
	if c := string(dlsCount("t", f)); c != "3\x00" {
		t.Fatalf("count %q", c)
	}
	l := string(dlsList("t", f))
	lines := strings.Split(strings.TrimSuffix(l, "\r\n"), "\r\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "97\t") || !strings.HasSuffix(lines[0], "\t8") {
		t.Fatalf("list %q", l)
	}
	text = "Elo 1016 changed" // a result arrived: the cached text stays until newsTTL
	f["contents"] = []byte("97")
	if b := string(dlsContents("t", f)); b != "Elo 1000" {
		t.Fatalf("contents %q", b)
	}
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	now = now.Add(newsTTL)
	if b := string(dlsContents("t", f)); b != "Elo 1016 changed" {
		t.Fatalf("after ttl %q", b)
	}
	f["contents"] = []byte("70")
	if b := string(dlsContents("t", f)); b != "admin news" {
		t.Fatalf("admin news %q", b)
	}
	// other console / no userid: only the disk files
	g := map[string][]byte{"rhgamecd": []byte("VDEE"), "userid": []byte("7"), "token": []byte(dlsIssueToken(7))}
	if c := string(dlsCount("t", g)); c != "2\x00" {
		t.Fatalf("other count %q", c)
	}
	delete(g, "userid")
	if c := string(dlsCount("t", g)); c != "2\x00" {
		t.Fatalf("no userid count %q", c)
	}
}

// #77: mail items (165..180) pass the generated-item filter, other ids >= 100 do not.
func TestNewsMailServed(t *testing.T) {
	old, oldSrc := dlcDir, ffcbonus.NewsSource
	dlcDir = t.TempDir()
	newsCache = map[string]newsEntry{}
	defer func() { dlcDir, ffcbonus.NewsSource = old, oldSrc }()
	ffcbonus.NewsSource = func(gamecd string, user uint64) []ffcbonus.NewsItem {
		return []ffcbonus.NewsItem{{Name: "165", Body: []byte("MAIL 1/1\nhi")}, {Name: "181", Body: []byte("x")}, {Name: "150", Body: []byte("x")}}
	}
	f := map[string][]byte{"rhgamecd": []byte("VDEE"), "userid": []byte("43"), "token": []byte(dlsIssueToken(43))}
	if l := string(dlsList("t", f)); l == "" || !strings.HasPrefix(l, "165\t") || strings.Count(l, "\r\n") != 1 {
		t.Fatalf("list %q", l)
	}
}
