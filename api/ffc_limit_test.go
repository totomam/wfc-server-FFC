package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"wwfc/ffcstats"
)

func TestFFCRateLimit(t *testing.T) {
	t0 := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	now := t0
	ffcNow = func() time.Time { return now }
	defer func() { ffcNow = time.Now }()
	ffcBuckets = map[string]*ffcBucket{}
	for i := 0; i < ffcRateBurst; i++ {
		if !ffcAllow("1.2.3.4") {
			t.Fatal("burst denied at", i)
		}
	}
	if ffcAllow("1.2.3.4") {
		t.Fatal("over burst allowed")
	}
	if !ffcAllow("5.6.7.8") {
		t.Fatal("other client limited")
	}
	now = t0.Add(time.Second) // 60/min -> 1 token per second
	if !ffcAllow("1.2.3.4") || ffcAllow("1.2.3.4") {
		t.Fatal("refill after 1 s must give exactly one request")
	}
	h := ffcLimited(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	req := httptest.NewRequest("GET", "/api/ffc/leaderboard", nil)
	req.RemoteAddr = "1.2.3.4:5555"
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" {
		t.Fatal("limited handler:", rec.Code)
	}
	req.RemoteAddr = "9.9.9.9:1"
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != 204 {
		t.Fatal("fresh client:", rec.Code)
	}
	if ffcClientIP(req) != "9.9.9.9" {
		t.Fatal(ffcClientIP(req))
	}
}

// NET-30: caller-chosen windows must not each replay the match table.
func TestFFCMatchCacheBoundedKeys(t *testing.T) {
	t0 := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	ffcNow = func() time.Time { return t0 }
	defer func() { ffcNow = time.Now }()
	all := []ffcstats.Match{}
	for i := 0; i < 500; i++ {
		all = append(all, ffcstats.Match{ID: int64(i), Status: "confirmed", CreatedAt: t0.Add(-time.Duration(i) * 7 * time.Hour)})
	}
	calls := 0
	old := ffcLoadMatchesDB
	defer func() { ffcLoadMatchesDB = old; ffcCache = map[[2]int64]ffcCacheEntry{} }()
	ffcLoadMatchesDB = func(from, to time.Time, limit int) ([]ffcstats.Match, error) {
		calls++
		out := []ffcstats.Match{}
		for _, m := range all {
			if (from.IsZero() || !m.CreatedAt.Before(from)) && (to.IsZero() || m.CreatedAt.Before(to)) {
				out = append(out, m)
			}
		}
		return out, nil
	}
	ffcCache = map[[2]int64]ffcCacheEntry{}
	for i := 0; i < 1000; i++ {
		from := t0.Add(-time.Duration(i)*13*time.Hour - time.Duration(i)*time.Nanosecond)
		to := from.Add(time.Duration(1+i%50) * 24 * time.Hour)
		got, err := ffcLoadMatchesCached(from, to)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		for _, m := range all {
			if !m.CreatedAt.Before(from) && m.CreatedAt.Before(to) {
				want++
			}
		}
		if len(got) != want { // the cache must return exactly the requested window
			t.Fatalf("window %d: %d rows, want %d", i, len(got), want)
		}
	}
	t.Logf("1000 distinct windows -> %d table loads", calls)
	if calls > ffcCacheKeysMax+1 {
		t.Fatalf("1000 distinct windows replayed the match table %d times (max %d)", calls, ffcCacheKeysMax+1)
	}
	if len(ffcCache) > ffcCacheKeysMax+1 {
		t.Fatalf("cache holds %d keys", len(ffcCache))
	}
}
