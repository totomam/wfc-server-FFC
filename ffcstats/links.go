package ffcstats

// ApplyLinks (ROADMAP #53): rewrite alias profile ids to their canonical profile (pid_a, pid_b, winner) so every
// stat counts one person once. links maps alias -> canonical (one level, see database/player_links.go). A row that
// becomes a self-match (both sides the same person) is dropped. The input slice is not modified.
func ApplyLinks(ms []Match, links map[uint32]uint32) []Match {
	if len(links) == 0 {
		return ms
	}
	out := make([]Match, 0, len(ms))
	for _, m := range ms {
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

// Resolve returns the canonical profile id of pid (pid itself when it is not an alias).
func Resolve(links map[uint32]uint32, pid uint32) uint32 {
	if c, ok := links[pid]; ok {
		return c
	}
	return pid
}
