// Package ffclimit: per-IP token buckets for the game protocols (ffc-decomp ROADMAP #69, protocol part; the public
// /api/ffc/* limit is api/ffc_limit.go). Limits are generous: several consoles behind one home NAT share an IP, and a
// two-console Battle with Anyone pair from one IP must never hit them (measured in s102-online, see the decomp
// docs/sessions/s102-online/protocol_limits.md). Over the limit: UDP packets are dropped, a TCP connection is
// closed, an HTTP request gets 503. Every limited event is counted; the first per IP per minute is logged.
package ffclimit

import (
	"container/list"
	"hash/fnv"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// Rule: Burst tokens, refilled at PerSec.
type Rule struct {
	Name   string
	Burst  float64
	PerSec float64
}

var (
	GPCMConnect = Rule{"gpcm_connect", 20, 1.0 / 3} // new GP connections per IP (20, then 1 per 3 s)
	GPCMPacket  = Rule{"gpcm_packet", 200, 20}      // GP TCP packets per IP
	GPCMLogin   = Rule{"gpcm_login", 20, 1.0 / 3}   // GP login commands per IP
	SBPacket    = Rule{"sb_packet", 120, 10}        // server-browser packets per IP (a searcher asks every few s)
	QR2Packet   = Rule{"qr2_packet", 600, 100}      // QR2 UDP packets per IP (heartbeats, keepalives, match commands)
	NATNEG      = Rule{"natneg_packet", 400, 60}    // NATNEG UDP packets per IP
	SAKE        = Rule{"sake_request", 120, 10}     // SAKE HTTP requests per IP
	NASRequest  = Rule{"nas_request", 120, 10}      // NAS (ac / download / FFC actions) HTTP requests per IP
	Disabled    atomic.Bool                         // true = every Allow returns true (emergency switch, tests)
	Now         = time.Now
	maxKeys     = 50000
)

// idleExpiry: a bucket not used for this long is full again anyway, so it is dropped.
const idleExpiry = 5 * time.Minute

const shardCount = 64

type bucket struct {
	key    string
	tokens float64
	last   time.Time
	logged time.Time
	elem   *list.Element
}

// shard: one lock, one map and one list ordered by last use (front = oldest), so expiry and eviction are O(1)
// per call instead of a full-map scan under one global lock (NET-09).
type shard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	lru     list.List
}

var (
	shards   [shardCount]shard
	countsMu sync.Mutex
	counts   = map[string]uint64{}
)

func init() {
	for i := range shards {
		shards[i].buckets = map[string]*bucket{}
	}
}

func shardOf(key string) *shard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &shards[h.Sum32()%shardCount]
}

// IPOf strips the port from "ip:port" (IPv4 or [IPv6]:port); other strings are returned unchanged.
func IPOf(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

// Allow takes one token from rule's bucket for ip. false = over the limit (caller drops / closes).
func Allow(r Rule, ip string) bool {
	if Disabled.Load() {
		return true
	}
	now := Now()
	key := r.Name + "|" + ip
	sh := shardOf(key)
	sh.mu.Lock()
	// Drop idle buckets from the old end (amortised O(1)).
	for e := sh.lru.Front(); e != nil; e = sh.lru.Front() {
		old := e.Value.(*bucket)
		if now.Sub(old.last) <= idleExpiry {
			break
		}
		sh.lru.Remove(e)
		delete(sh.buckets, old.key)
	}
	b, ok := sh.buckets[key]
	if !ok {
		// Full shard: evict the least recently used bucket. That bucket restarts full, which only ever lets more
		// through, never less.
		if len(sh.buckets) >= maxKeys/shardCount {
			if e := sh.lru.Front(); e != nil {
				sh.lru.Remove(e)
				delete(sh.buckets, e.Value.(*bucket).key)
			}
		}
		b = &bucket{key: key, tokens: r.Burst, last: now}
		b.elem = sh.lru.PushBack(b)
		sh.buckets[key] = b
	} else {
		sh.lru.MoveToBack(b.elem)
	}
	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens += elapsed.Seconds() * r.PerSec
	}
	if b.tokens > r.Burst {
		b.tokens = r.Burst
	}
	if now.After(b.last) {
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		sh.mu.Unlock()
		return true
	}
	logIt := now.Sub(b.logged) > time.Minute
	if logIt {
		b.logged = now
	}
	sh.mu.Unlock()
	countsMu.Lock()
	counts[r.Name]++
	countsMu.Unlock()
	if logIt {
		logging.Warn("LIMIT", aurora.Yellow(r.Name), "over the limit for", aurora.BrightCyan(ip))
	}
	return false
}

// Keys returns how many buckets are kept (tests, dashboard).
func Keys() int {
	n := 0
	for i := range shards {
		shards[i].mu.Lock()
		n += len(shards[i].buckets)
		shards[i].mu.Unlock()
	}
	return n
}

// Counts returns how many events each rule has refused since start (for the admin dashboard / tests).
func Counts() map[string]uint64 {
	countsMu.Lock()
	defer countsMu.Unlock()
	out := make(map[string]uint64, len(counts))
	for k, v := range counts {
		out[k] = v
	}
	return out
}

// Reset clears all state (tests).
func Reset() {
	for i := range shards {
		shards[i].mu.Lock()
		shards[i].buckets = map[string]*bucket{}
		shards[i].lru.Init()
		shards[i].mu.Unlock()
	}
	countsMu.Lock()
	counts = map[string]uint64{}
	countsMu.Unlock()
}
