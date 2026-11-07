package gpcm

// OnlinePlayer is one logged-in GPCM session (ffc-decomp ROADMAP #30 live player counter, #72 admin dashboard).
type OnlinePlayer struct {
	ProfileID  uint32 `json:"pid"`
	InGameName string `json:"name"`
	Status     string `json:"status"` // GP status string the game last set (FFC: "|s|<n>|..."), "" if none
}

// OnlinePlayers lists the logged-in sessions of one game (gamename, e.g. "dental2ds"); "" = every game.
func OnlinePlayers(gameName string) []OnlinePlayer {
	mutex.Lock()
	defer mutex.Unlock()
	out := []OnlinePlayer{}
	for pid, s := range sessions {
		if !s.LoggedIn || (gameName != "" && s.GameName != gameName) {
			continue
		}
		out = append(out, OnlinePlayer{ProfileID: pid, InGameName: s.InGameName, Status: s.Status})
	}
	return out
}
