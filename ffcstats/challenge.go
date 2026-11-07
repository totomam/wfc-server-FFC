package ffcstats

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// Challenge: a daily / weekly goal (ffc-decomp ROADMAP #73, server part; design note
// docs/sessions/s102-online/challenges_news.md in the decomp repo). Admin-written challenges.json; progress is
// computed from confirmed match_results in [Start, End) -- nothing is stored. Kinds:
//
//	games       play Target matches          wins       win Target matches
//	games_with  ... with Creature in my team  wins_with  ... with Creature in my team
//
// Queue: "" / ranked (default) | casual | all (FilterMode). Modded: nil any | true | false.
type Challenge struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Kind     string    `json:"kind"`
	Creature uint8     `json:"creature,omitempty"`
	Target   int       `json:"target"`
	Queue    string    `json:"queue,omitempty"`
	Modded   *bool     `json:"modded,omitempty"`
	Reward   string    `json:"reward,omitempty"` // free text shown on the web/Discord (rewards are given by admins)
}

const maxChallenges = 64

func ParseChallenges(r io.Reader) ([]Challenge, error) {
	var cs []Challenge
	if err := json.NewDecoder(r).Decode(&cs); err != nil {
		return nil, err
	}
	if len(cs) > maxChallenges {
		return nil, fmt.Errorf("at most %d challenges", maxChallenges)
	}
	seen := map[string]bool{}
	for i, c := range cs {
		if c.ID == "" || seen[c.ID] {
			return nil, fmt.Errorf("challenge %d: missing or duplicate id", i)
		}
		seen[c.ID] = true
		if c.Start.IsZero() || c.End.IsZero() || !c.End.After(c.Start) {
			return nil, fmt.Errorf("challenge %s: start and end required, end after start", c.ID)
		}
		switch c.Kind {
		case "games", "wins":
		case "games_with", "wins_with":
			if c.Creature == 0 {
				return nil, fmt.Errorf("challenge %s: %s needs creature", c.ID, c.Kind)
			}
		default:
			return nil, fmt.Errorf("challenge %s: unknown kind %q", c.ID, c.Kind)
		}
		if c.Target < 1 || c.Target > 1000 {
			return nil, fmt.Errorf("challenge %s: target 1..1000", c.ID)
		}
		switch c.Queue {
		case "", "ranked", "casual", "all":
		default:
			return nil, fmt.Errorf("challenge %s: queue ranked|casual|all", c.ID)
		}
	}
	return cs, nil
}

// ChallengeProgress is one player's count toward a challenge.
type ChallengeProgress struct {
	Pid   uint32     `json:"pid"`
	Name  string     `json:"name,omitempty"`
	Count int        `json:"count"`
	Done  bool       `json:"done"`
	At    *time.Time `json:"completed_at,omitempty"` // the match that reached Target
}

// ChallengeResult: the challenge, whether it is running at `now`, and every player with progress (done first, then
// by count, then earliest completion).
type ChallengeResult struct {
	Challenge
	Active    bool                `json:"active"`
	Completed int                 `json:"completed"`
	Players   []ChallengeProgress `json:"players"`
}

func hasCreature(team []Pick, id uint8) bool {
	for _, p := range team {
		if p.Creature == id {
			return true
		}
	}
	return false
}

// EvalChallenge counts progress over ms (already confirmed + links applied; any window: filtered here).
func EvalChallenge(c Challenge, ms []Match, now time.Time) ChallengeResult {
	ms = FilterMode(Filter(ms, c.Modded, c.Start, c.End, nil), c.Queue)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].CreatedAt.Before(ms[j].CreatedAt) })
	prog := map[uint32]*ChallengeProgress{}
	count := func(pid uint32, name string, team []Pick, at time.Time, won bool) {
		if pid == 0 {
			return
		}
		switch c.Kind {
		case "wins":
			if !won {
				return
			}
		case "games_with":
			if !hasCreature(team, c.Creature) {
				return
			}
		case "wins_with":
			if !won || !hasCreature(team, c.Creature) {
				return
			}
		}
		p := prog[pid]
		if p == nil {
			p = &ChallengeProgress{Pid: pid}
			prog[pid] = p
		}
		if name != "" {
			p.Name = name
		}
		if p.Done {
			return
		}
		p.Count++
		if p.Count >= c.Target {
			p.Done = true
			t := at
			p.At = &t
		}
	}
	for _, m := range ms {
		count(m.PidA, m.NameA, m.TeamA, m.CreatedAt, !m.Draw && m.WinnerPid == m.PidA)
		count(m.PidB, m.NameB, m.TeamB, m.CreatedAt, !m.Draw && m.WinnerPid == m.PidB)
	}
	r := ChallengeResult{Challenge: c, Active: !now.Before(c.Start) && now.Before(c.End), Players: []ChallengeProgress{}}
	for _, p := range prog {
		r.Players = append(r.Players, *p)
		if p.Done {
			r.Completed++
		}
	}
	sort.Slice(r.Players, func(i, j int) bool {
		a, b := r.Players[i], r.Players[j]
		if a.Done != b.Done {
			return a.Done
		}
		if a.Done && !a.At.Equal(*b.At) {
			return a.At.Before(*b.At)
		}
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Pid < b.Pid
	})
	return r
}
