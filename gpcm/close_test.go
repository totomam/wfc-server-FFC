package gpcm

import (
	"fmt"
	"testing"
	"wwfc/database"
)

// TestCloseConnectionFreesIndex (O1-7): every closed connection, logged in or
// not, must leave sessionsByConnIndex.
func TestCloseConnectionFreesIndex(t *testing.T) {
	startFakeFrontend(t)
	mutex.Lock()
	before := len(sessionsByConnIndex)
	mutex.Unlock()

	for i := uint64(0); i < 1000; i++ {
		idx := 500000 + i
		// One address per connection so per-IP connect limits do not refuse it
		NewConnection(idx, fmt.Sprintf("10.%d.%d.1:1234", i/256, i%256))
		if i%100 == 0 {
			// Some connections log in before closing.
			mutex.Lock()
			s := sessionsByConnIndex[idx]
			if s == nil {
				mutex.Unlock()
				t.Fatalf("connection %d was not registered", idx)
			}
			s.LoggedIn = true
			s.User = database.User{ProfileId: uint32(800000 + i)}
			sessions[s.User.ProfileId] = s
			mutex.Unlock()
		}
		CloseConnection(idx)
		takeSent(idx)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if n := len(sessionsByConnIndex) - before; n != 0 {
		t.Fatalf("len(sessionsByConnIndex) grew by %d after 1000 connect/close cycles", n)
	}
	for i := uint32(0); i < 1000; i += 100 {
		if _, ok := sessions[800000+i]; ok {
			t.Fatalf("logged-in session %d not removed", 800000+i)
		}
	}
}
