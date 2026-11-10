package qr2

import (
	"testing"
	"time"
	"wwfc/common"
)

// NET-10 (#71, decision c): a QR2 join remembers the pair; the memory outlives the group (a disconnect after grouping
// still counts) for PairWindow after they were last together; strangers are never "seen together".
func TestRecentPairs(t *testing.T) {
	now := time.Unix(100000, 0)
	pairNow = func() time.Time { return now }
	defer func() { pairNow = time.Now }()
	ForgetPairs()
	defer ForgetPairs()

	host := &Session{Data: map[string]string{"gamename": "pairtest", "dwc_mtype": "0", "dwc_pid": "5001"}}
	guest := &Session{Data: map[string]string{"gamename": "pairtest", "dwc_pid": "5002"}}
	if SeenTogether(5001, 5002) {
		t.Fatal("strangers seen together")
	}
	mutex.Lock()
	ok := processResvOK("t", 3, common.MatchCommandDataReservation{}, common.MatchCommandDataResvOK{GroupID: 77}, host, guest)
	mutex.Unlock()
	if !ok || !SeenTogether(5001, 5002) || !SeenTogether(5002, 5001) {
		t.Fatal("pair not remembered on join")
	}
	if SeenTogether(5001, 5003) {
		t.Fatal("third profile seen")
	}

	// Long match: the join is older than the window, but both are still grouped -> still a pair.
	now = now.Add(PairWindow + time.Minute)
	if !SeenTogether(5001, 5002) {
		t.Fatal("live group not counted")
	}
	// The guest disconnects: remembered from this moment, still valid after the group is gone.
	mutex.Lock()
	guest.removeFromGroup()
	host.removeFromGroup()
	mutex.Unlock()
	now = now.Add(PairWindow - time.Minute)
	if !SeenTogether(5001, 5002) {
		t.Fatal("pair forgotten inside the window after a disconnect")
	}
	now = now.Add(2 * time.Minute)
	if SeenTogether(5001, 5002) {
		t.Fatal("pair still remembered after the window")
	}
}
