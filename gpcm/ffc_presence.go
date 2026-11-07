package gpcm

// FFC online-friends feed (ffc-decomp ROADMAP #56, human 2026-11-07: Discord): webhook events "player_online" (GPCM
// login done) and "player_offline" (logged-in connection closed). Unlike the upstream "logged_in"/"logged_out"
// events they carry no IP address, so they are safe for a public Discord channel. Off unless a webhook lists them in
// config.xml <eventTypes> (or "all", which also sends the IP events). Fields: player name, friend code, the player's
// mutual friends online now (names), players online now, and the session length on logout. The offline event waits
// presenceGrace (60 s): a re-login inside it is a reconnect and sends nothing.

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wwfc/common"
	"wwfc/logging"
)

var (
	presenceMutex sync.Mutex
	presenceSince = map[uint32]time.Time{}
	presenceNow   = time.Now
	// presenceGrace: a logout followed by a login of the same profile within this window is a reconnect (FFC logs in
	// twice when it opens the Wi-Fi menu, ~1 s apart): neither event is sent and the session keeps its start time.
	presenceGrace   = 60 * time.Second
	presencePending = map[uint32]*time.Timer{}
)

// presenceName is the label a feed line shows: the in-game name, else the profile id.
func presenceName(s *GameSpySession) string {
	if name := strings.TrimSpace(s.InGameName); name != "" {
		return name
	}
	return strconv.FormatUint(uint64(s.User.ProfileId), 10)
}

// mutualFriendsOnline lists (sorted) the names of logged-in players of the same game that are on s's friend list and
// have s on theirs. Caller must hold mutex.
func mutualFriendsOnline(s *GameSpySession) []string {
	me := s.User.ProfileId
	out := []string{}
	for _, pid := range s.FriendList {
		o, ok := sessions[pid]
		if !ok || o == s || !o.LoggedIn || o.GameName != s.GameName {
			continue
		}
		for _, back := range o.FriendList {
			if back == me {
				out = append(out, presenceName(o))
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// onlineCount counts logged-in sessions of one game, skipping one profile. Caller must hold mutex.
func onlineCount(gameName string, skip uint32) int {
	n := 0
	for pid, o := range sessions {
		if o.LoggedIn && o.GameName == gameName && pid != skip {
			n++
		}
	}
	return n
}

func presenceData(s *GameSpySession, online int, friends []string) map[string]any {
	fc := ""
	if len(s.User.GsbrCode) >= 4 {
		fc = common.CalcFriendCodeString(s.User.ProfileId, s.User.GsbrCode[:4])
	}
	f := "none"
	if len(friends) > 0 {
		f = strings.Join(friends, ", ")
	}
	return map[string]any{
		"Player":         presenceName(s),
		"Friend code":    fc,
		"Friends online": f,
		"Players online": online,
		"Game":           s.GameName,
	}
}

// presenceOnline is called once the login reply is sent (session registered, LoggedIn set).
func (g *GameSpySession) presenceOnline() {
	mutex.Lock()
	friends := mutualFriendsOnline(g)
	online := onlineCount(g.GameName, 0)
	mutex.Unlock()

	presenceMutex.Lock()
	if t, ok := presencePending[g.User.ProfileId]; ok && t.Stop() {
		delete(presencePending, g.User.ProfileId)
		presenceMutex.Unlock()
		return // reconnect inside the grace window
	}
	presenceSince[g.User.ProfileId] = presenceNow()
	presenceMutex.Unlock()

	logging.Event("player_online", presenceData(g, online, friends))
}

// presenceOffline is called when a logged-in connection closes, before the session is removed.
func (g *GameSpySession) presenceOffline() {
	mutex.Lock()
	friends := mutualFriendsOnline(g)
	online := onlineCount(g.GameName, g.User.ProfileId)
	mutex.Unlock()

	data := presenceData(g, online, friends)
	pid := g.User.ProfileId
	send := func() {
		presenceMutex.Lock()
		delete(presencePending, pid)
		if t, ok := presenceSince[pid]; ok {
			data["Online for"] = presenceDuration(presenceNow().Sub(t))
			delete(presenceSince, pid)
		}
		presenceMutex.Unlock()
		logging.Event("player_offline", data)
	}
	if presenceGrace <= 0 {
		send()
		return
	}
	presenceMutex.Lock()
	if old, ok := presencePending[pid]; ok {
		old.Stop()
	}
	presencePending[pid] = time.AfterFunc(presenceGrace, send)
	presenceMutex.Unlock()
}

// presenceDuration formats a session length as "42m" or "1h05m".
func presenceDuration(d time.Duration) string {
	m := int(d.Round(time.Minute) / time.Minute)
	if m < 60 {
		return strconv.Itoa(m) + "m"
	}
	return strconv.Itoa(m/60) + "h" + strconv.Itoa(m%60/10) + strconv.Itoa(m%10) + "m"
}
