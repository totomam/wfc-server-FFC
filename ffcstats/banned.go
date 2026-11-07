package ffcstats

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Banned-vivosaur lists per queue (ffc-decomp ROADMAP #75, server part, s103-online). Admin-written banned.json; a
// confirmed result whose team (either side) holds a banned creature in a matching match is not counted in the public
// stats (leaderboard, Elo, challenges, rewards, news) and is listed for admins (/api/ffc/admin/banned). The game itself
// is not stopped from playing it (a client-side refusal needs the package; docs/online/NEWS.md "#75").
//
//	{"id":"no-bonus-ranked","queue":"ranked","modded":null,"creatures":[199,203],"start":"...","end":"..."}
//
// Queue: "" / ranked (default) | casual | all. Modded: nil any | true | false. Start/End: optional window.
type BanRule struct {
	ID        string     `json:"id"`
	Queue     string     `json:"queue,omitempty"`
	Modded    *bool      `json:"modded,omitempty"`
	Creatures []int      `json:"creatures"`
	Start     *time.Time `json:"start,omitempty"`
	End       *time.Time `json:"end,omitempty"`
}

const maxBanRules = 64

func ParseBanRules(r io.Reader) ([]BanRule, error) {
	var rs []BanRule
	if err := json.NewDecoder(r).Decode(&rs); err != nil {
		return nil, err
	}
	if len(rs) > maxBanRules {
		return nil, fmt.Errorf("at most %d rules", maxBanRules)
	}
	for _, b := range rs {
		switch b.Queue {
		case "", "ranked", "casual", "all":
		default:
			return nil, fmt.Errorf("rule %q: bad queue %q", b.ID, b.Queue)
		}
		if b.ID == "" || len(b.Creatures) == 0 {
			return nil, fmt.Errorf("rule %q: id and creatures required", b.ID)
		}
		if b.Start != nil && b.End != nil && !b.End.After(*b.Start) {
			return nil, fmt.Errorf("rule %q: end before start", b.ID)
		}
	}
	return rs, nil
}

func (b BanRule) applies(m Match) bool {
	if b.Modded != nil && *b.Modded != m.Modded {
		return false
	}
	if b.Queue != "all" && (m.Queue == "casual") != (b.Queue == "casual") {
		return false
	}
	if b.Start != nil && m.CreatedAt.Before(*b.Start) || b.End != nil && !m.CreatedAt.Before(*b.End) {
		return false
	}
	return true
}

// Violation: one match that breaks a rule, with the side(s) and creature(s) that did.
type Violation struct {
	Match Match    `json:"match"`
	Rule  string   `json:"rule"`
	Pids  []uint32 `json:"pids"`
	Found []int    `json:"creatures"`
}

func violation(b BanRule, m Match) (Violation, bool) {
	if !b.applies(m) {
		return Violation{}, false
	}
	v := Violation{Match: m, Rule: b.ID}
	side := func(pid uint32, team []Pick) {
		hit := false
		for _, p := range team {
			for _, c := range b.Creatures {
				if int(p.Creature) == c {
					v.Found = append(v.Found, c)
					hit = true
				}
			}
		}
		if hit {
			v.Pids = append(v.Pids, pid)
		}
	}
	side(m.PidA, m.TeamA)
	side(m.PidB, m.TeamB)
	return v, len(v.Pids) > 0
}

// Violations lists every (match, first broken rule).
func Violations(ms []Match, rules []BanRule) []Violation {
	out := []Violation{}
	for _, m := range ms {
		for _, b := range rules {
			if v, ok := violation(b, m); ok {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// DropBanned returns ms without the matches that break a rule (ms itself is not modified).
func DropBanned(ms []Match, rules []BanRule) []Match {
	if len(rules) == 0 {
		return ms
	}
	out := make([]Match, 0, len(ms))
next:
	for _, m := range ms {
		for _, b := range rules {
			if _, ok := violation(b, m); ok {
				continue next
			}
		}
		out = append(out, m)
	}
	return out
}
