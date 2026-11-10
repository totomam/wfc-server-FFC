package gpcm

import (
	"strings"
	"testing"
	"wwfc/common"
	"wwfc/database"
)

// dentalPair logs in two mutual dental2ds friends: sender (pid 9001, conn 91)
// and receiver (pid 9002, conn 92).
func dentalPair(t *testing.T) (*GameSpySession, *GameSpySession) {
	t.Helper()
	startFakeFrontend(t)
	g := &GameSpySession{ConnIndex: 91, LoggedIn: true, LoginInfoSet: true, DeviceAuthenticated: true, GameName: "dental2ds",
		ModuleName: "test", User: database.User{ProfileId: 9001},
		FriendList: []uint32{9002}, AuthFriendList: []uint32{9002}}
	r := &GameSpySession{ConnIndex: 92, LoggedIn: true, LoginInfoSet: true, DeviceAuthenticated: true, GameName: "dental2ds",
		ModuleName: "test", User: database.User{ProfileId: 9002},
		FriendList: []uint32{9001}, AuthFriendList: []uint32{9001}}
	mutex.Lock()
	sessions[9001], sessions[9002] = g, r
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		delete(sessions, 9001)
		delete(sessions, 9002)
		mutex.Unlock()
		takeSent(91)
		takeSent(92)
	})
	takeSent(91)
	takeSent(92)
	return g, r
}

func statusCmd(stat, loc string) common.GameSpyCommand {
	return common.GameSpyCommand{Command: "status", CommandValue: "1",
		OtherValues: map[string]string{"statstring": stat, "locstring": loc}}
}

// TestFFCStatusCaps (NET-39 a): over-long /KEY/value values and names longer
// than 8 chars in the locstring are not relayed to a dental2ds friend.
func TestFFCStatusCaps(t *testing.T) {
	cases := []struct {
		name, stat, loc string
		relayed         bool
	}{
		{"honest", "/SCM/1/SCN/0/VER/90/LCK/0", "MO_1_NAME_HUNTER", true},
		{"honest-8-char-name", "/SCM/2/SCN/1/VER/90/LCK/1", "MO_0_NAME_ABCDEFGH", true},
		{"empty", "", "", true},
		{"long-LCK", "/SCM/1/SCN/0/VER/90/LCK/" + strings.Repeat("A", 12), "MO_1_NAME_HUNTER", false},
		{"4-char-value", "/LCK/1234", "MO_1_NAME_HUNTER", false},
		{"9-char-name", "/VER/90", "MO_1_NAME_ABCDEFGHI", false},
		{"bad-loc-shape", "/VER/90", "MO_X_NAME_A", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, _ := dentalPair(t)
			g.setStatus(statusCmd(c.stat, c.loc))
			sent := takeSent(92)
			if got := len(sent) > 0; got != c.relayed {
				t.Fatalf("relayed=%v, want %v (sent %q)", got, c.relayed, sent)
			}
			if c.relayed && !strings.Contains(sent[0], c.loc) {
				t.Fatalf("relayed status %q lacks locstring %q", sent[0], c.loc)
			}
		})
	}
}

// TestFFCBuddyMessagePinned (NET-39 c): only GPCM3vMAT/GPCM11vMAT/GPCM90vMAT
// buddy messages are accepted; "GPCM90" + 20 non-'v' bytes is rejected and
// nothing reaches the friend.
func TestFFCBuddyMessagePinned(t *testing.T) {
	for _, msg := range []string{
		"GPCM90" + strings.Repeat("A", 20),
		"GPCM3" + strings.Repeat("x", 20),
		"GPCM90vMAT\x03" + strings.Repeat("!", 20), // bad base64 payload
	} {
		g, _ := dentalPair(t)
		g.bestieMessage(common.GameSpyCommand{Command: "bm", CommandValue: "1",
			OtherValues: map[string]string{"t": "9002", "msg": msg}})
		if sent := takeSent(92); len(sent) != 0 {
			t.Fatalf("message %q relayed: %q", msg, sent)
		}
		if g.WriteBuffer != "" {
			t.Fatalf("message %q queued a reply %q", msg, g.WriteBuffer)
		}
		if sent := takeSent(91); len(sent) == 0 || !strings.Contains(sent[0], "\\error\\") {
			t.Fatalf("message %q: sender got no error, sent %q", msg, sent)
		}
	}
}

// TestFFCStatusRelayGuard: a sender that is not itself a dental2ds session
// still cannot push an over-long status to a dental2ds friend.
func TestFFCStatusRelayGuard(t *testing.T) {
	g, _ := dentalPair(t)
	g.GameName = "othergame"
	g.setStatus(statusCmd("/LCK/"+strings.Repeat("A", 12), "MO_1_NAME_HUNTER"))
	if sent := takeSent(92); len(sent) != 0 {
		t.Fatalf("over-long status relayed to dental2ds friend: %q", sent)
	}
}
