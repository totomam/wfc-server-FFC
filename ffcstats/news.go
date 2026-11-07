package ffcstats

import (
	"fmt"
	"sort"
	"strings"
)

// News texts for the in-game Bonus Data box (ffc-decomp ROADMAP #79 profile, #41 top players, #73 challenges;
// s103-online, docs/online/NEWS.md). Plain ASCII, <= 0x3ff bytes. Empty string = no item. s104-online melonDS: the box
// wraps by itself at ~37 characters and grows to ~10 visible rows, with no paging (rows past the screen are lost), so
// lines stay <= newsCols and items <= newsRows rows.

const (
	newsMax  = 0x3ff
	newsCols = 36 // measured ~37-38 (s104-online)
	newsRows = 10
)

func newsName(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < 10; i++ {
		if c := s[i]; c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return b.String()
}

func newsCut(s string) string {
	if len(s) > newsMax {
		s = s[:newsMax]
	}
	return s
}

// ProfileNews: one player's row of a Leaderboard (rows), "" when they have no match in it.
func ProfileNews(rows []Player, pid uint32, season string) string {
	for _, p := range rows {
		if p.Pid != pid {
			continue
		}
		head := "YOUR RANKED STATS"
		if season != "" {
			head += "\n(season " + newsName(season) + ")"
		}
		return newsCut(fmt.Sprintf("%s\n%s  #%d\nElo %d (best %d)\nW %d  L %d  D %d", head, newsName(p.Name), p.Rank,
			int(p.Elo+0.5), int(p.Peak+0.5), p.Wins, p.Losses, p.Draws))
	}
	return ""
}

// TopNews: the first n rows of a Leaderboard.
func TopNews(rows []Player, n int) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("TOP PLAYERS (ranked)")
	for i, p := range rows {
		if i >= n {
			break
		}
		fmt.Fprintf(&b, "\n%d. %s %d", p.Rank, newsName(p.Name), int(p.Elo+0.5))
	}
	return newsCut(b.String())
}

// ChallengesNews: the active challenges with pid's progress.
func ChallengesNews(rs []ChallengeResult, pid uint32) string {
	var b strings.Builder
	for _, r := range rs {
		if !r.Active {
			continue
		}
		cnt, done := 0, false
		for _, p := range r.Players {
			if p.Pid == pid {
				cnt, done = p.Count, p.Done
			}
		}
		if cnt > r.Target {
			cnt = r.Target
		}
		line := fmt.Sprintf("\n%s\n %d/%d", newsTitle(r.Title, r.ID), cnt, r.Target)
		if done {
			line += " DONE!"
		} else {
			line += " ends " + r.End.UTC().Format("Jan 2")
		}
		if b.Len()+len(line)+len("CHALLENGES") > newsMax || strings.Count(b.String()+line, "\n")+1 > newsRows {
			break
		}
		b.WriteString(line)
	}
	if b.Len() == 0 {
		return ""
	}
	return "CHALLENGES" + b.String()
}

func newsTitle(title, id string) string {
	if title == "" {
		title = id
	}
	var b strings.Builder
	for i := 0; i < len(title) && b.Len() < newsCols; i++ {
		if c := title[i]; c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// RecentNews (#42 text replays, s105-online): pid's last 3 confirmed matches (any queue), newest first, as
// "WIN/LOSS/DRAW vs <name> (<queue>)" + both teams by vivosaur name (vivo: id -> short name; missing -> "#id").
// 1 + 3 x 3 rows <= newsRows; each team row is cut to newsCols. ms must already have #53 links applied.
func RecentNews(ms []Match, pid uint32, vivo map[int]string) string {
	var mine []Match
	for _, m := range ms {
		if m.Status == "confirmed" && (m.PidA == pid || m.PidB == pid) && m.PidA != m.PidB {
			mine = append(mine, m)
		}
	}
	if len(mine) == 0 {
		return ""
	}
	sort.SliceStable(mine, func(i, j int) bool { return mine[i].CreatedAt.After(mine[j].CreatedAt) })
	team := func(label string, t []Pick) string {
		ps := append([]Pick(nil), t...)
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Slot < ps[j].Slot })
		s := label
		for i, p := range ps {
			n, ok := vivo[int(p.Creature)]
			if !ok || n == "" {
				n = fmt.Sprintf("#%d", p.Creature)
			}
			if i > 0 {
				s += ","
			}
			s += newsName(n)
		}
		if len(s) > newsCols {
			s = s[:newsCols]
		}
		return s
	}
	var b strings.Builder
	b.WriteString("YOUR LAST MATCHES")
	for i, m := range mine {
		if i >= 3 {
			break
		}
		opp, myT, oppT := m.NameB, m.TeamA, m.TeamB
		if m.PidB == pid {
			opp, myT, oppT = m.NameA, m.TeamB, m.TeamA
		}
		res := "LOSS"
		switch {
		case m.Draw || m.WinnerPid == 0:
			res = "DRAW"
		case m.WinnerPid == pid:
			res = "WIN"
		}
		q := m.Queue
		if q == "" {
			q = "ranked"
		}
		fmt.Fprintf(&b, "\n%s vs %s (%s)", res, newsName(opp), newsName(q))
		b.WriteString("\n" + team(" you: ", myT))
		b.WriteString("\n" + team(" foe: ", oppT))
	}
	return newsCut(b.String())
}
