package qr2

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestGetSessionServersConcurrent (O1-6): readers iterate the maps returned by
// GetSessionServers while SetDeviceAuthenticated writes the live session.Data.
// Run with -race: the returned maps must be copies.
func TestGetSessionServersConcurrent(t *testing.T) {
	const pid = 7101
	key := uint64(0x0a000001) | (uint64(0x2222) << 32)
	s := &Session{Authenticated: true, LastKeepAlive: time.Now().UTC().Unix(),
		Data: map[string]string{"gamename": "dental2ds", "dwc_pid": strconv.Itoa(pid), "+deviceauth": "0"}}
	mutex.Lock()
	sessions[key] = s
	logins[pid] = &LoginInfo{ProfileID: pid, session: s}
	mutex.Unlock()
	defer func() {
		mutex.Lock()
		delete(sessions, key)
		delete(logins, pid)
		mutex.Unlock()
	}()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				for _, server := range GetSessionServers() {
					for k, v := range server {
						_, _ = k, v
					}
					_ = server["+deviceauth"]
				}
			}
		}()
	}
	for i := 0; i < 2000; i++ {
		SetDeviceAuthenticated(pid)
		mutex.Lock()
		s.Data["+deviceauth"] = "0"
		s.Data["k"+strconv.Itoa(i%8)] = "v"
		mutex.Unlock()
	}
	close(stop)
	wg.Wait()

	// The copy is a snapshot: changing it does not touch the session.
	servers := GetSessionServers()
	for _, server := range servers {
		if server["dwc_pid"] == strconv.Itoa(pid) {
			server["+deviceauth"] = "x"
		}
	}
	mutex.Lock()
	defer mutex.Unlock()
	if s.Data["+deviceauth"] == "x" {
		t.Fatal("GetSessionServers returned the live session.Data map")
	}
}
