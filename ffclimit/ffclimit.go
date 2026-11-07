// Package ffclimit: per-IP token buckets for the game protocols (ffc-decomp ROADMAP #69, protocol part; the public
// /api/ffc/* limit is api/ffc_limit.go). Limits are generous: several consoles behind one home NAT share an IP, and a
// two-console Battle with Anyone pair from one IP must never hit them (measured in s102-online, see the decomp
// docs/sessions/s102-online/protocol_limits.md). Over the limit: UDP packets are dropped, a TCP connection is
// closed, an HTTP request gets 503. Every limited event is counted; the first per IP per minute is logged.
package ffclimit

import (
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

type bucket struct {
	tokens float64
	last   time.Time
	logged time.Time
}

var (
	mu      sync.Mutex
	buckets = map[string]*bucket{}
	counts  = map[string]uint64{}
)

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
	mu.Lock()
	b, ok := buckets[key]
	if !ok {
		if len(buckets) >= maxKeys {
			for k, v := range buckets {
				if now.Sub(v.last) > 5*time.Minute {
					delete(buckets, k)
				}
			}
		}
		b = &bucket{tokens: r.Burst, last: now}
		buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * r.PerSec
	if b.tokens > r.Burst {
		b.tokens = r.Burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		mu.Unlock()
		return true
	}
	counts[r.Name]++
	logIt := now.Sub(b.logged) > time.Minute
	if logIt {
		b.logged = now
	}
	mu.Unlock()
	if logIt {
		logging.Warn("LIMIT", aurora.Yellow(r.Name), "over the limit for", aurora.BrightCyan(ip))
	}
	return false
}

// Counts returns how many events each rule has refused since start (for the admin dashboard / tests).
func Counts() map[string]uint64 {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]uint64, len(counts))
	for k, v := range counts {
		out[k] = v
	}
	return out
}

// Reset clears all state (tests).
func Reset() {
	mu.Lock()
	buckets, counts = map[string]*bucket{}, map[string]uint64{}
	mu.Unlock()
}
