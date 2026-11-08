package ffcstats

// ApplyLinks (ROADMAP #53): rewrite alias profile ids to their canonical profile (pid_a, pid_b, winner) so every
// stat counts one person once. links maps alias -> canonical (one level, see database/player_links.go). A row that
// becomes a self-match (both sides the same person) is dropped. The input slice is not modified.
func ApplyLinks(ms []Match, links map[uint32]uint32) []Match {
	var names map[uint32]string
	if Names != nil {
		names = Names()
	}
	if len(links) == 0 && len(names) == 0 {
		return ms
	}
	out := make([]Match, 0, len(ms))
	for _, m := range ms {
		m.NameA, m.NameB = lbName(names, links, m.PidA, m.NameA), lbName(names, links, m.PidB, m.NameB)
		m.PidA, m.PidB = Resolve(links, m.PidA), Resolve(links, m.PidB)
		if m.WinnerPid != 0 {
			m.WinnerPid = Resolve(links, m.WinnerPid)
		}
		if m.PidA == m.PidB {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Names (ROADMAP #52, s113-online): the leaderboard names players typed in game (database ffc_lb_names), set by
// the api package. ApplyLinks puts them in NameA/NameB, so every stats view + the widget shows them. nil = none.
var Names func() map[uint32]string

// lbName: the canonical profile's typed name, else the profile's own, else the save's name from the report.
func lbName(names map[uint32]string, links map[uint32]uint32, pid uint32, name string) string {
	if n, ok := names[Resolve(links, pid)]; ok {
		return n
	}
	if n, ok := names[pid]; ok {
		return n
	}
	return name
}

// Resolve returns the canonical profile id of pid (pid itself when it is not an alias).
func Resolve(links map[uint32]uint32, pid uint32) uint32 {
	if c, ok := links[pid]; ok {
		return c
	}
	return pid
}
