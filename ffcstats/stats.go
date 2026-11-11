// Package ffcstats computes the FFC PvP leaderboard and analytics from match_results rows (ffc-decomp ROADMAP
// #21 win/loss, #26 Elo, #22 pick-rate, #23 win-rate, #25 head-to-head, #27 seasons = date window, #32 match log).
// Pure functions over already-loaded rows: the caller picks the window (season), the queue (modded/vanilla) and
// which statuses count (by default only "confirmed" = both consoles agreed, #28).
package ffcstats

import (
	"encoding/json"
	"math"
	"sort"
	"time"
)

// Match is one match_results row (only the fields the stats need).
type Match struct {
	ID        int64     `json:"id"`
	PidA      uint32    `json:"pid_a"`
	PidB      uint32    `json:"pid_b"`
	WinnerPid uint32    `json:"winner_pid"` // 0 = draw
	Draw      bool      `json:"draw"`
	TimeUp    bool      `json:"time_up"`
	Modded    bool      `json:"modded"`
	MatchVer  uint32    `json:"match_ver"` // dwc_mver of the match: 90 vanilla, else 0x40000000|rules crc (#29)
	NameA     string    `json:"name_a"`
	NameB     string    `json:"name_b"`
	TeamA     []Pick    `json:"team_a"`
	TeamB     []Pick    `json:"team_b"`
	Status    string    `json:"status"`
	Queue     string    `json:"queue"` // ranked | casual | "" unknown (#35)
	CreatedAt time.Time `json:"created_at"`
	// NET-35: the profile whose console sent a "single" row's only report, and whether the server had seen the pair
	// in one QR2 group (NET-10) when it was stored. Not public.
	Reporter uint32 `json:"-"`
	PairSeen bool   `json:"-"`
}

// Pick is one vivosaur of a team (subset of nas.ResultCreature's JSON).
type Pick struct {
	Slot     int   `json:"slot"`
	Creature uint8 `json:"creature"`
	Rank     uint8 `json:"rank_index"`
}

// ParseTeam decodes a team_a/team_b jsonb value; bad JSON = empty team.
func ParseTeam(raw []byte) []Pick {
	var t []Pick
	if json.Unmarshal(raw, &t) != nil {
		return nil
	}
	return t
}

const (
	EloStart = 1000.0
	EloK     = 32.0
)

// Player is one leaderboard row.
type Player struct {
	Pid     uint32  `json:"pid"`
	Name    string  `json:"name"` // name of the player's most recent match
	Wins    int     `json:"wins"`
	Losses  int     `json:"losses"`
	Draws   int     `json:"draws"`
	Played  int     `json:"played"`
	WinRate float64 `json:"win_rate"` // wins / played (draws count as played)
	Elo     float64 `json:"elo"`
	Peak    float64 `json:"elo_peak"`
	Rank    int     `json:"rank"`
}

func sortChrono(ms []Match) []Match {
	out := append([]Match(nil), ms...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// score of A: 1 win, 0 loss, 0.5 draw; ok=false for a row whose winner is neither player.
func scoreA(m Match) (float64, bool) {
	switch {
	case m.Draw || m.WinnerPid == 0:
		return 0.5, true
	case m.WinnerPid == m.PidA:
		return 1, true
	case m.WinnerPid == m.PidB:
		return 0, true
	}
	return 0, false
}

// Leaderboard replays the matches in time order: W/L/D and Elo (start 1000, K 32, draws = 0.5). Sorted by Elo,
// then wins, then pid; minPlayed hides players with fewer matches (they still affect their opponents' Elo).
func Leaderboard(ms []Match, minPlayed int) []Player {
	byPid := map[uint32]*Player{}
	get := func(pid uint32, name string) *Player {
		p := byPid[pid]
		if p == nil {
			p = &Player{Pid: pid, Elo: EloStart, Peak: EloStart}
			byPid[pid] = p
		}
		if name != "" {
			p.Name = name
		}
		return p
	}
	for _, m := range sortChrono(ms) {
		if m.PidA == m.PidB {
			continue
		}
		sa, ok := scoreA(m)
		if !ok {
			continue
		}
		a, b := get(m.PidA, m.NameA), get(m.PidB, m.NameB)
		ea := 1 / (1 + math.Pow(10, (b.Elo-a.Elo)/400))
		d := EloK * (sa - ea)
		a.Elo += d
		b.Elo -= d
		a.Peak, b.Peak = math.Max(a.Peak, a.Elo), math.Max(b.Peak, b.Elo)
		a.Played++
		b.Played++
		switch sa {
		case 1:
			a.Wins++
			b.Losses++
		case 0:
			a.Losses++
			b.Wins++
		default:
			a.Draws++
			b.Draws++
		}
	}
	out := []Player{}
	for _, p := range byPid {
		if p.Played < minPlayed {
			continue
		}
		p.WinRate = float64(p.Wins) / float64(p.Played)
		p.Elo = math.Round(p.Elo*10) / 10
		p.Peak = math.Round(p.Peak*10) / 10
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Elo != out[j].Elo {
			return out[i].Elo > out[j].Elo
		}
		if out[i].Wins != out[j].Wins {
			return out[i].Wins > out[j].Wins
		}
		return out[i].Pid < out[j].Pid
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// Vivosaur is one pick/win-rate row (#22/#23). A creature listed twice in one team counts once for that team.
type Vivosaur struct {
	Creature uint8   `json:"creature"`
	Picks    int     `json:"picks"`     // teams that used it
	PickRate float64 `json:"pick_rate"` // picks / teams
	Wins     int     `json:"wins"`
	Losses   int     `json:"losses"`
	Draws    int     `json:"draws"`
	WinRate  float64 `json:"win_rate"` // wins / picks
}

// Vivosaurs aggregates every team of every counted match. Sorted by picks, then creature id.
func Vivosaurs(ms []Match) (rows []Vivosaur, teams int) {
	by := map[uint8]*Vivosaur{}
	add := func(team []Pick, s float64) {
		seen := map[uint8]bool{}
		for _, c := range team {
			if seen[c.Creature] {
				continue
			}
			seen[c.Creature] = true
			v := by[c.Creature]
			if v == nil {
				v = &Vivosaur{Creature: c.Creature}
				by[c.Creature] = v
			}
			v.Picks++
			switch s {
			case 1:
				v.Wins++
			case 0:
				v.Losses++
			default:
				v.Draws++
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
		teams += 2
	}
	rows = []Vivosaur{}
	for _, v := range by {
		v.PickRate = float64(v.Picks) / float64(teams)
		v.WinRate = float64(v.Wins) / float64(v.Picks)
		rows = append(rows, *v)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Picks != rows[j].Picks {
			return rows[i].Picks > rows[j].Picks
		}
		return rows[i].Creature < rows[j].Creature
	})
	return rows, teams
}

// HeadToHead is #25: the record of pid a against pid b, plus their matches (newest first).
type HeadToHead struct {
	A, B    uint32  `json:"-"`
	WinsA   int     `json:"wins_a"`
	WinsB   int     `json:"wins_b"`
	Draws   int     `json:"draws"`
	Matches []Match `json:"matches"`
}

func H2H(ms []Match, a, b uint32) HeadToHead {
	h := HeadToHead{A: a, B: b, Matches: []Match{}}
	for _, m := range ms {
		if !((m.PidA == a && m.PidB == b) || (m.PidA == b && m.PidB == a)) {
			continue
		}
		sa, ok := scoreA(m)
		if !ok {
			continue
		}
		switch {
		case sa == 0.5:
			h.Draws++
		case (sa == 1) == (m.PidA == a):
			h.WinsA++
		default:
			h.WinsB++
		}
		h.Matches = append(h.Matches, m)
	}
	sort.SliceStable(h.Matches, func(i, j int) bool { return h.Matches[i].CreatedAt.After(h.Matches[j].CreatedAt) })
	return h
}

// Filter keeps the rows of one queue and window: modded nil = both queues; zero times = open-ended; statuses
// empty = only "confirmed".
// FilterMode (s99 #35): "" / "ranked" = every match except friend battles (unknown counts as ranked, so rows from
// before the queue column stay on the leaderboard); "casual" = friend battles only; "all" = no filter.
func FilterMode(ms []Match, mode string) []Match {
	if mode == "all" {
		return ms
	}
	out := []Match{}
	for _, m := range ms {
		if (m.Queue == "casual") == (mode == "casual") {
			out = append(out, m)
		}
	}
	return out
}

// A "forfeit" row (ApplyForfeits) counts wherever "confirmed" does.
func Filter(ms []Match, modded *bool, from, to time.Time, statuses []string) []Match {
	if len(statuses) == 0 {
		statuses = []string{"confirmed"}
	}
	ok := map[string]bool{}
	for _, s := range statuses {
		ok[s] = true
	}
	ok[StatusForfeit] = ok[StatusForfeit] || ok["confirmed"]
	out := []Match{}
	for _, m := range ms {
		if !ok[m.Status] || (modded != nil && m.Modded != *modded) {
			continue
		}
		if (!from.IsZero() && m.CreatedAt.Before(from)) || (!to.IsZero() && !m.CreatedAt.Before(to)) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// StatusForfeit (NET-35, human decision 2026-11-10: a quit counts as a loss) is the read-time status of a "single"
// row (only one console reported) that is older than the forfeit timeout: the side that did not report lost.
const StatusForfeit = "forfeit"

// ApplyForfeits turns every "single" row older than after (at now) into a counted forfeit row: Status = StatusForfeit,
// WinnerPid = Reporter, Draw = false, unless the report admits the reporter's own loss (admitsLoss: kept as reported). Only rows of a pair the server saw in one QR2 group (PairSeen,
// NET-10) qualify, so a made-up report against a stranger never becomes a win; rows from before that check never do.
// Two single rows of one pair from opposite reporters less than after apart (each console reported, but too late to
// be paired into one row) contradict each other: neither becomes a forfeit. after <= 0 disables forfeits.
// The rows are copied when one changes; ms itself is not modified.
func ApplyForfeits(ms []Match, after time.Duration, now time.Time) []Match {
	if after <= 0 {
		return ms
	}
	var out []Match
	for i, m := range ms {
		if !forfeitable(m, after, now) || contradicted(ms, i, after) {
			continue
		}
		if out == nil {
			out = append([]Match(nil), ms...)
		}
		out[i].Status = StatusForfeit
		if !admitsLoss(m) {
			out[i].WinnerPid, out[i].Draw = m.Reporter, false
		}
	}
	if out == nil {
		return ms
	}
	return out
}

// admitsLoss: the lone report says its own console lost (the other side won, then left). That result is kept: the
// forfeit rule only makes the side that did not report lose, never the reporter a winner against its own report.
func admitsLoss(m Match) bool {
	other := m.PidA
	if m.Reporter == m.PidA {
		other = m.PidB
	}
	return !m.Draw && m.WinnerPid == other
}

func forfeitable(m Match, after time.Duration, now time.Time) bool {
	return m.Status == "single" && m.PairSeen && (m.Reporter == m.PidA || m.Reporter == m.PidB) && m.Reporter != 0 &&
		now.Sub(m.CreatedAt) >= after
}

func contradicted(ms []Match, i int, after time.Duration) bool {
	m := ms[i]
	for j, o := range ms {
		if j == i || o.Status != "single" || o.PidA != m.PidA || o.PidB != m.PidB || o.Reporter == m.Reporter {
			continue
		}
		if d := o.CreatedAt.Sub(m.CreatedAt); d < after && d > -after {
			return true
		}
	}
	return false
}
