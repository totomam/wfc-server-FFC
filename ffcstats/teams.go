package ffcstats

import (
	"sort"
	"strconv"
	"strings"
)

// Team / build usage (ffc-decomp ROADMAP #24): a team = the set of creature ids fielded (slot order and duplicates
// ignored, so A+B == B+A); a pair = two different creatures on the same team (synergy view). Same scoring as
// Vivosaurs (win 1, loss 0, draw 0.5 counted as draw).
type TeamUsage struct {
	Creatures []uint8 `json:"creatures"`
	Picks     int     `json:"picks"`
	Wins      int     `json:"wins"`
	Losses    int     `json:"losses"`
	Draws     int     `json:"draws"`
	PickRate  float64 `json:"pick_rate"`
	WinRate   float64 `json:"win_rate"`
}

func teamKey(team []Pick) ([]uint8, string) {
	seen := map[uint8]bool{}
	ids := []uint8{}
	for _, p := range team {
		if p.Creature != 0 && !seen[p.Creature] {
			seen[p.Creature] = true
			ids = append(ids, p.Creature)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for i, c := range ids {
		parts[i] = strconv.Itoa(int(c))
	}
	return ids, strings.Join(parts, ",")
}

// Teams returns team compositions and creature pairs with at least min picks, most picked first.
func Teams(ms []Match, min int) (teams []TeamUsage, pairs []TeamUsage, total int) {
	tb, pb := map[string]*TeamUsage{}, map[string]*TeamUsage{}
	bump := func(by map[string]*TeamUsage, key string, ids []uint8, s float64) {
		u := by[key]
		if u == nil {
			u = &TeamUsage{Creatures: ids}
			by[key] = u
		}
		u.Picks++
		switch s {
		case 1:
			u.Wins++
		case 0:
			u.Losses++
		default:
			u.Draws++
		}
	}
	add := func(team []Pick, s float64) {
		ids, key := teamKey(team)
		if len(ids) == 0 {
			return
		}
		total++
		bump(tb, key, ids, s)
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				pair := []uint8{ids[i], ids[j]}
				bump(pb, strconv.Itoa(int(ids[i]))+","+strconv.Itoa(int(ids[j])), pair, s)
			}
		}
	}
	for _, m := range ms {
		sa, ok := scoreA(m)
		if !ok || m.PidA == m.PidB {
			continue
		}
		add(m.TeamA, sa)
		add(m.TeamB, 1-sa)
	}
	flat := func(by map[string]*TeamUsage) []TeamUsage {
		out := []TeamUsage{}
		for _, u := range by {
			if u.Picks < min {
				continue
			}
			u.PickRate = float64(u.Picks) / float64(total)
			u.WinRate = float64(u.Wins) / float64(u.Picks)
			out = append(out, *u)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Picks != out[j].Picks {
				return out[i].Picks > out[j].Picks
			}
			a, b := out[i].Creatures, out[j].Creatures
			for k := 0; k < len(a) && k < len(b); k++ {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
			return len(a) < len(b)
		})
		return out
	}
	return flat(tb), flat(pb), total
}
