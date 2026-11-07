package api

import (
	"net"
	"net/http"
	"sync"
	"time"
	"wwfc/ffcstats"
)

// Abuse limits for the public FFC API (ffc-decomp ROADMAP #69): every /api/ffc/* read replays the match table, so
// (1) a per-client-IP token bucket (ffcRateBurst requests, refilled at ffcRatePerMin per minute; 429 when empty) and
// (2) a short cache of the loaded rows per time window, shared by all clients. Admin endpoints (secret) are not limited.
// The client IP is the TCP peer (no X-Forwarded-For trust: the NAS is not behind a proxy on the FFC VPS).

const (
	ffcRateBurst  = 20
	ffcRatePerMin = 60
	ffcCacheTTL   = 15 * time.Second
	ffcBucketsMax = 10000 // forget idle clients past this many
)

type ffcBucket struct {
	tokens float64
	last   time.Time
}

var (
	ffcBucketsMu sync.Mutex
	ffcBuckets   = map[string]*ffcBucket{}
	ffcNow       = time.Now
)

func ffcClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ffcAllow takes one token for the client; false = over the limit.
func ffcAllow(ip string) bool {
	now := ffcNow()
	ffcBucketsMu.Lock()
	defer ffcBucketsMu.Unlock()
	b, ok := ffcBuckets[ip]
	if !ok {
		if len(ffcBuckets) >= ffcBucketsMax {
			for k, v := range ffcBuckets {
				if now.Sub(v.last) > time.Minute {
					delete(ffcBuckets, k)
				}
			}
		}
		b = &ffcBucket{tokens: ffcRateBurst, last: now}
		ffcBuckets[ip] = b
	}
	b.tokens += now.Sub(b.last).Minutes() * ffcRatePerMin
	if b.tokens > ffcRateBurst {
		b.tokens = ffcRateBurst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func ffcLimited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions && !ffcAllow(ffcClientIP(r)) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Retry-After", "5")
			replyError(w, http.StatusTooManyRequests, "rate_limited")
			return
		}
		h(w, r)
	}
}

type ffcCacheEntry struct {
	at   time.Time
	rows []ffcstats.Match
}

var (
	ffcCacheMu sync.Mutex
	ffcCache   = map[[2]int64]ffcCacheEntry{}
)

// ffcLoadMatchesCached: db.LoadMatches for the window, reused for ffcCacheTTL. Callers only filter/read the rows.
func ffcLoadMatchesCached(from, to time.Time) ([]ffcstats.Match, error) {
	key := [2]int64{from.UnixNano(), to.UnixNano()}
	now := ffcNow()
	ffcCacheMu.Lock()
	if e, ok := ffcCache[key]; ok && now.Sub(e.at) < ffcCacheTTL {
		ffcCacheMu.Unlock()
		return e.rows, nil
	}
	ffcCacheMu.Unlock()
	rows, err := db.LoadMatches(from, to, ffcMaxRows)
	if err != nil {
		return nil, err
	}
	ffcCacheMu.Lock()
	for k, e := range ffcCache {
		if now.Sub(e.at) >= ffcCacheTTL {
			delete(ffcCache, k)
		}
	}
	ffcCache[key] = ffcCacheEntry{at: now, rows: rows}
	ffcCacheMu.Unlock()
	return rows, nil
}
