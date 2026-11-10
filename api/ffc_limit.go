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
	ffcRateBurst    = 20
	ffcRatePerMin   = 60
	ffcCacheTTL     = 15 * time.Second
	ffcCacheKeysMax = 16             // NET-30: live window keys before a miss loads the whole table
	ffcCacheRound   = 24 * time.Hour // NET-30: cached windows are rounded out to whole UTC days
	ffcBucketsMax   = 10000          // forget idle clients past this many
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
	at       time.Time
	from, to time.Time // loaded window, rounded to ffcCacheRound; zero = open end
	rows     []ffcstats.Match
}

var (
	ffcCacheMu sync.Mutex
	ffcCache   = map[[2]int64]ffcCacheEntry{}
	ffcLoadMu  sync.Mutex // NET-30: one table load at a time; waiters re-check the cache
	// ffcLoadMatchesDB is db.LoadMatches; tests replace it to count table replays.
	ffcLoadMatchesDB = func(from, to time.Time, limit int) ([]ffcstats.Match, error) { return db.LoadMatches(from, to, limit) }
)

// ffcCovers: entry e holds every row of [from, to) (and was not cut by the row limit).
func ffcCovers(e ffcCacheEntry, from, to time.Time) bool {
	if len(e.rows) >= ffcMaxRows {
		return false
	}
	okFrom := e.from.IsZero() || (!from.IsZero() && !from.Before(e.from))
	okTo := e.to.IsZero() || (!to.IsZero() && !to.After(e.to))
	return okFrom && okTo
}

// ffcCacheLookup returns a live entry covering [from, to); caller holds ffcCacheMu.
func ffcCacheLookup(now, from, to time.Time) (ffcCacheEntry, bool) {
	for k, e := range ffcCache {
		if now.Sub(e.at) >= ffcCacheTTL {
			delete(ffcCache, k)
			continue
		}
		if ffcCovers(e, from, to) {
			return e, true
		}
	}
	return ffcCacheEntry{}, false
}

// ffcWindowRows: the rows of e inside [from, to) (zero = open end), as db.LoadMatches(from, to) would return them.
func ffcWindowRows(e ffcCacheEntry, from, to time.Time) []ffcstats.Match {
	if (from.IsZero() || from.Equal(e.from)) && (to.IsZero() || to.Equal(e.to)) {
		return e.rows
	}
	out := make([]ffcstats.Match, 0, len(e.rows))
	for _, m := range e.rows {
		if (!from.IsZero() && m.CreatedAt.Before(from)) || (!to.IsZero() && !m.CreatedAt.Before(to)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ffcLoadMatchesCached: db.LoadMatches for the window minus banned.json violations (#75), reused for ffcCacheTTL.
// Callers only filter/read the rows.
// NET-30: the window is caller-chosen (query params, now+1h in brackets), so the cache key is the window rounded out to
// ffcCacheRound, any live entry that covers the window answers it, and past ffcCacheKeysMax live keys a miss loads the
// whole table (one entry that covers every window). So at most ffcCacheKeysMax+1 loads per ffcCacheTTL, whatever the
// windows (unless the table exceeds ffcMaxRows, where a covering load would be cut and every load is exact again).
func ffcLoadMatchesCached(from, to time.Time) ([]ffcstats.Match, error) {
	now := ffcNow()
	ffcCacheMu.Lock()
	e, ok := ffcCacheLookup(now, from, to)
	ffcCacheMu.Unlock()
	if ok {
		return ffcWindowRows(e, from, to), nil
	}
	ffcLoadMu.Lock()
	defer ffcLoadMu.Unlock()
	ffcCacheMu.Lock()
	e, ok = ffcCacheLookup(now, from, to)
	full := len(ffcCache) >= ffcCacheKeysMax
	ffcCacheMu.Unlock()
	if ok {
		return ffcWindowRows(e, from, to), nil
	}
	lf, lt := time.Time{}, time.Time{}
	if !full {
		if !from.IsZero() {
			lf = from.UTC().Truncate(ffcCacheRound)
		}
		if !to.IsZero() {
			lt = to.UTC().Truncate(ffcCacheRound)
			if lt.Before(to) {
				lt = lt.Add(ffcCacheRound)
			}
		}
	}
	rows, err := ffcLoadMatchesDB(lf, lt, ffcMaxRows)
	if err == nil && len(rows) >= ffcMaxRows && !(lf.Equal(from) && lt.Equal(to)) {
		lf, lt = from, to // cut by the row limit: a wider load would drop rows of the window, so load it exactly
		rows, err = ffcLoadMatchesDB(lf, lt, ffcMaxRows)
	}
	if err != nil {
		return nil, err
	}
	rows = ffcstats.DropBanned(rows, ffcBanRules()) // #75: banned-team matches never reach public stats
	e = ffcCacheEntry{at: now, from: lf, to: lt, rows: rows}
	ffcCacheMu.Lock()
	if len(ffcCache) >= ffcCacheKeysMax+1 { // only reachable if the full-table load was cut by ffcMaxRows
		for k := range ffcCache {
			delete(ffcCache, k)
			break
		}
	}
	ffcCache[[2]int64{lf.UnixNano(), lt.UnixNano()}] = e
	ffcCacheMu.Unlock()
	return ffcWindowRows(e, from, to), nil
}
