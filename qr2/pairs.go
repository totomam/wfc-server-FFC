package qr2

import (
	"encoding/gob"
	"os"
	"strconv"
	"sync"
	"time"
	"wwfc/common"
)

// Recent pairs (ffc-decomp NET-10 #71, human decision 2026-11-10 c): the server remembers which profiles were in one
// QR2 group together, so a PvP result report (nas/dls_result.go) for a pair that never met is refused. A pair is
// remembered from the moment one joins the other's group and refreshed when either leaves it, so a disconnect after
// grouping still counts (NET-35: the leaver's missing report becomes a loss). Entries expire after PairWindow.
// In memory: nas and qr2 run in the same backend process; a backend reload keeps it (state/qr2_pairs.gob).

// DefaultPairWindow is how long a pair stays remembered after it was last seen together (config.xml
// <resultPairWindowMinutes>).
const DefaultPairWindow = 30 * time.Minute

type pairKey struct{ a, b uint32 }

var (
	pairMutex  sync.Mutex
	recentPair = map[pairKey]time.Time{}
	PairWindow = DefaultPairWindow
	pairNow    = time.Now
)

func makePairKey(a, b uint32) pairKey {
	if a > b {
		a, b = b, a
	}
	return pairKey{a, b}
}

// RememberPair records that profiles a and b were grouped together now (0 or a == b: ignored).
func RememberPair(a, b uint32) {
	if a == 0 || b == 0 || a == b {
		return
	}
	now := pairNow()
	pairMutex.Lock()
	defer pairMutex.Unlock()
	recentPair[makePairKey(a, b)] = now
	if len(recentPair)%1024 == 0 { // occasional sweep keeps the map bounded by the window
		for k, at := range recentPair {
			if now.Sub(at) > PairWindow {
				delete(recentPair, k)
			}
		}
	}
}

// SeenTogether: a and b were in one QR2 group within the last PairWindow (or are in one now, see notePairsOf).
func SeenTogether(a, b uint32) bool {
	pairMutex.Lock()
	at, ok := recentPair[makePairKey(a, b)]
	pairMutex.Unlock()
	if ok && pairNow().Sub(at) <= PairWindow {
		return true
	}
	return inSharedGroup(a, b)
}

// ForgetPairs clears the memory (tests).
func ForgetPairs() {
	pairMutex.Lock()
	defer pairMutex.Unlock()
	recentPair = map[pairKey]time.Time{}
}

func pidOf(s *Session) uint32 {
	if s == nil {
		return 0
	}
	return parsePid(s.Data["dwc_pid"])
}

func parsePid(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}

// notePairsOf remembers s with every other player of its group. Expects the qr2 mutex to be locked.
func notePairsOf(s *Session, g *Group) {
	if g == nil {
		return
	}
	me := pidOf(s)
	for p := range g.players {
		if p != s {
			RememberPair(me, pidOf(p))
		}
	}
}

// inSharedGroup: a and b are in one live group (a match longer than PairWindow with nobody leaving).
func inSharedGroup(a, b uint32) bool {
	mutex.Lock()
	defer mutex.Unlock()
	for _, g := range groups {
		var hasA, hasB bool
		for p := range g.players {
			pid := pidOf(p)
			hasA = hasA || pid == a
			hasB = hasB || pid == b
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

func savePairs() error {
	file, err := os.OpenFile("state/qr2_pairs.gob", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer func() {
		common.ShouldNotError(file.Close())
	}()
	pairMutex.Lock()
	defer pairMutex.Unlock()
	m := map[[2]uint32]time.Time{}
	for k, at := range recentPair {
		m[[2]uint32{k.a, k.b}] = at
	}
	return gob.NewEncoder(file).Encode(m)
}

// loadPairs: a missing file (first start after the upgrade) is not an error.
func loadPairs() error {
	file, err := os.Open("state/qr2_pairs.gob")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() {
		common.ShouldNotError(file.Close())
	}()
	m := map[[2]uint32]time.Time{}
	if err := gob.NewDecoder(file).Decode(&m); err != nil {
		return err
	}
	pairMutex.Lock()
	defer pairMutex.Unlock()
	for k, at := range m {
		recentPair[pairKey{k[0], k[1]}] = at
	}
	return nil
}
