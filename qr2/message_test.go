package qr2

import (
	"encoding/binary"
	"testing"
)

// TestSendClientMessageBadRating (O1-5): a match command from a session whose
// rating is invalid takes the checkValidRating branch of setProfileID, which
// unlocks and re-locks the global mutex. SendClientMessage must hold the mutex
// there, or the process dies with "sync: unlock of unlocked mutex".
func TestSendClientMessageBadRating(t *testing.T) {
	const (
		senderPID = 7001
		qr2IP     = 0x01020304 // 1.2.3.4
		qr2Port   = 0x1234
		searchID  = 77
	)
	var gotPID uint32
	var gotErr string
	old := gpErrorCallback
	gpErrorCallback = func(pid uint32, e string) {
		gotPID, gotErr = pid, e
		// The callback runs with the mutex released: it must be lockable here.
		mutex.Lock()
		mutex.Unlock()
	}
	defer func() { gpErrorCallback = old }()

	receiver := &Session{Authenticated: true, Data: map[string]string{"dwc_pid": "7002", "gamename": "mariokartwii"},
		login: &LoginInfo{ProfileID: 7002, DeviceAuthenticated: true}}
	sender := &Session{Authenticated: true, Data: map[string]string{"gamename": "mariokartwii", "rk": "vs", "ev": "0"}}
	senderKey := (uint64(qr2Port) << 32) | uint64(qr2IP)

	mutex.Lock()
	sessionBySearchID[searchID] = receiver
	sessions[senderKey] = sender
	logins[senderPID] = &LoginInfo{ProfileID: senderPID, GPPublicIP: "1.2.3.4:29900", DeviceAuthenticated: true}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		delete(sessionBySearchID, searchID)
		delete(sessions, senderKey)
		delete(logins, senderPID)
		mutex.Unlock()
	}()

	msg := make([]byte, 0x14)
	copy(msg, "SBCM")
	binary.LittleEndian.PutUint32(msg[0x04:], 3)
	msg[8] = 0x01 // match command
	msg[9] = 0    // no payload
	binary.LittleEndian.PutUint16(msg[0x0A:], qr2Port)
	binary.BigEndian.PutUint32(msg[0x0C:], qr2IP)
	binary.LittleEndian.PutUint32(msg[0x10:], senderPID)

	SendClientMessage("1.2.3.4:5000", searchID, msg)

	if gotErr != "invalid_elo" || gotPID != senderPID {
		t.Fatalf("gpErrorCallback = (%d, %q), want (%d, invalid_elo)", gotPID, gotErr, senderPID)
	}
	if sender.login != nil {
		t.Fatal("sender got a login despite the invalid rating")
	}
	// The mutex must be free again.
	mutex.Lock()
	mutex.Unlock()
}
