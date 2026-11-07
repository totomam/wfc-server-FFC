package gpcm

import (
	"sync"
	"testing"
	"time"

	"wwfc/database"
	"wwfc/logging"
)

func TestPresenceEvents(t *testing.T) {
	type ev struct {
		name string
		data map[string]any
	}
	ch := make(chan ev, 8)
	logging.RegisterEventCallback([]string{"player_online", "player_offline"}, func(e string, d map[string]any) {
		if d["Player"] != "Hunter" {
			return // callbacks stay registered: ignore the other tests
		}
		select {
		case ch <- ev{e, d}:
		default:
		}
	})
	clock := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	presenceNow = func() time.Time { return clock }
	presenceGrace = 0
	defer func() { presenceNow = time.Now; presenceGrace = 60 * time.Second }()

	a := &GameSpySession{LoggedIn: true, GameName: "dental2ds", InGameName: "Hunter", FriendList: []uint32{2, 3},
		User: database.User{ProfileId: 1, GsbrCode: "VDEEx"}}
	b := &GameSpySession{LoggedIn: true, GameName: "dental2ds", InGameName: "Rosie", FriendList: []uint32{1},
		User: database.User{ProfileId: 2}}
	c := &GameSpySession{LoggedIn: true, GameName: "dental2ds", InGameName: "OneWay", FriendList: nil,
		User: database.User{ProfileId: 3}}
	mutex.Lock()
	sessions[1], sessions[2], sessions[3] = a, b, c
	mutex.Unlock()
	defer func() { mutex.Lock(); delete(sessions, 1); delete(sessions, 2); delete(sessions, 3); mutex.Unlock() }()

	a.presenceOnline()
	clock = clock.Add(65 * time.Minute)
	a.presenceOffline()
	got := map[string]map[string]any{}
	for len(got) < 2 {
		e := <-ch
		got[e.name] = e.data
	}
	on, off := got["player_online"], got["player_offline"]
	if on["Player"] != "Hunter" || on["Friends online"] != "Rosie" || on["Players online"] != 3 {
		t.Fatalf("online %v", on)
	}
	if on["Friend code"] == "" {
		t.Fatalf("no friend code %v", on)
	}
	if _, ok := on["ip_address"]; ok {
		t.Fatal("ip leaked")
	}
	if off["Online for"] != "1h05m" || off["Players online"] != 2 {
		t.Fatalf("offline %v", off)
	}
	if presenceDuration(42*time.Minute) != "42m" {
		t.Fatal(presenceDuration(42 * time.Minute))
	}
}

func TestPresenceReconnect(t *testing.T) {
	var mu sync.Mutex
	n := map[string]int{}
	logging.RegisterEventCallback([]string{"player_online", "player_offline"}, func(ev string, d map[string]any) {
		if d["Player"] != "Dino" {
			return
		}
		mu.Lock()
		n[ev]++
		mu.Unlock()
	})
	presenceGrace = 50 * time.Millisecond
	defer func() { presenceGrace = 60 * time.Second }()
	g := &GameSpySession{LoggedIn: true, GameName: "dental2ds", InGameName: "Dino", User: database.User{ProfileId: 9}}
	g.presenceOnline()
	g.presenceOffline()
	g.presenceOnline() // reconnect inside the grace window: nothing sent
	time.Sleep(20 * time.Millisecond)
	g.presenceOffline()
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if n["player_online"] != 1 || n["player_offline"] != 1 {
		t.Fatalf("events %v", n)
	}
}
