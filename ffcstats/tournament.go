package ffcstats

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// Tournaments (ffc-decomp ROADMAP #45, server part): an admin-written tournaments.json next to the server lists
// single-elimination brackets; results are verified from match_results (#20 dual reports, "confirmed" rows only):
// a bracket match is decided by the first confirmed, non-draw match between its two players inside the tournament
// window and after both players' previous bracket match. No sign-up flow and no Discord bot here: the bot (or a
// web page) reads /api/ffc/tournaments and announces / assigns roles (#43 via /api/ffc/rewards).
//
// tournaments.json: [{"id": "cup1", "name": "November Cup", "start": "2026-11-14T18:00:00Z",
//
//	"end": "2026-11-15T00:00:00Z", "mode": "casual", "players": [1000000001, 1000000002, ...]}]
//
// players = seed order; pairs are (1,2), (3,4), ... in round 1; the list is padded with byes (0) to a power of two.
// mode = ranked | casual | all (default casual: tournament games are friend battles, #35).
type Tournament struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Mode    string    `json:"mode,omitempty"`
	Players []uint32  `json:"players"`
}

// BracketMatch is one pairing; Winner 0 = undecided. A bye is B = 0 (A advances, MatchID 0).
type BracketMatch struct {
	A       uint32     `json:"a"`
	B       uint32     `json:"b"`
	Winner  uint32     `json:"winner"`
	MatchID int64      `json:"match_id,omitempty"`
	At      *time.Time `json:"at,omitempty"`
}

type Bracket struct {
	Tournament
	Rounds   [][]BracketMatch `json:"rounds"`
	Champion uint32           `json:"champion"` // 0 = not finished
	Finished bool             `json:"finished"`
}

const maxTournamentPlayers = 256

func ParseTournaments(r io.Reader) ([]Tournament, error) {
	var ts []Tournament
	if err := json.NewDecoder(r).Decode(&ts); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for i, t := range ts {
		if t.ID == "" || seen[t.ID] {
			return nil, fmt.Errorf("tournament %d: missing or duplicate id", i)
		}
		seen[t.ID] = true
		if t.Start.IsZero() || (!t.End.IsZero() && !t.End.After(t.Start)) {
			return nil, fmt.Errorf("tournament %s: bad start/end", t.ID)
		}
		if len(t.Players) < 2 || len(t.Players) > maxTournamentPlayers {
			return nil, fmt.Errorf("tournament %s: 2..%d players", t.ID, maxTournamentPlayers)
		}
		dup := map[uint32]bool{}
		for _, p := range t.Players {
			if p == 0 || dup[p] {
				return nil, fmt.Errorf("tournament %s: zero or duplicate player %d", t.ID, p)
			}
			dup[p] = true
		}
		switch t.Mode {
		case "", "ranked", "casual", "all":
		default:
			return nil, fmt.Errorf("tournament %s: mode must be ranked, casual or all", t.ID)
		}
	}
	return ts, nil
}

func FindTournament(ts []Tournament, id string) (Tournament, bool) {
	for _, t := range ts {
		if t.ID == id {
			return t, true
		}
	}
	return Tournament{}, false
}

// Resolve the bracket from match rows (already status-filtered and link-resolved by the caller; players are
// canonical pids, #53). Rows outside [Start, End) or in another queue mode are ignored.
func ResolveBracket(t Tournament, ms []Match) Bracket {
	mode := t.Mode
	if mode == "" {
		mode = "casual"
	}
	var in []Match
	for _, m := range sortChrono(FilterMode(ms, mode)) {
		if m.CreatedAt.Before(t.Start) || (!t.End.IsZero() && !m.CreatedAt.Before(t.End)) {
			continue
		}
		if m.Draw || m.WinnerPid == 0 || m.PidA == m.PidB {
			continue
		}
		in = append(in, m)
	}
	n := 1
	for n < len(t.Players) {
		n *= 2
	}
	// slot: pid 0 + known = bye; pid 0 + !known = winner of an undecided match.
	type slot struct {
		pid   uint32
		known bool
	}
	cur := make([]slot, n)
	for i := range cur {
		cur[i].known = true
		if i < len(t.Players) {
			cur[i].pid = t.Players[i]
		}
	}
	ready := map[uint32]time.Time{} // a player's matches only count after their previous bracket win
	b := Bracket{Tournament: t}
	for len(cur) > 1 {
		var round []BracketMatch
		next := make([]slot, 0, len(cur)/2)
		for i := 0; i < len(cur); i += 2 {
			x, y := cur[i], cur[i+1]
			bm := BracketMatch{A: x.pid, B: y.pid}
			res := slot{}
			switch {
			case !x.known || !y.known:
				// an earlier match is undecided
			case x.pid == 0 && y.pid == 0:
				res = slot{0, true}
			case y.pid == 0:
				bm.Winner, res = x.pid, x
			case x.pid == 0:
				bm.A, bm.B, bm.Winner, res = y.pid, 0, y.pid, y
			default:
				after := ready[bm.A]
				if ready[bm.B].After(after) {
					after = ready[bm.B]
				}
				for _, m := range in {
					pair := (m.PidA == bm.A && m.PidB == bm.B) || (m.PidA == bm.B && m.PidB == bm.A)
					if pair && m.CreatedAt.After(after) && (m.WinnerPid == bm.A || m.WinnerPid == bm.B) {
						at := m.CreatedAt
						bm.Winner, bm.MatchID, bm.At = m.WinnerPid, m.ID, &at
						ready[m.WinnerPid] = m.CreatedAt
						res = slot{m.WinnerPid, true}
						break
					}
				}
			}
			round = append(round, bm)
			next = append(next, res)
		}
		b.Rounds = append(b.Rounds, round)
		cur = next
	}
	b.Champion = cur[0].pid
	b.Finished = cur[0].known && b.Champion != 0
	return b
}
