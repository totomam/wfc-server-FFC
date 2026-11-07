package api

import (
	"time"
	"wwfc/ffcbonus"
	"wwfc/ffcstats"
	"wwfc/logging"
)

// In-game news (ffc-decomp ROADMAP #79 profile, #41 top players, #73 challenges; s103-online, docs/online/NEWS.md):
// three Bonus Data files generated per console for the Nd list/contents (ffcbonus.NewsSource). A modded-package game shows
// them on every Bonus Data visit; a stock game skips ids > 64 (nothing changes for it).
//
//	97 profile:    the player's ranked row (current season, else all time): rank, Elo, W/L/D
//	98 top:        top 5 of the same leaderboard (players with >= 5 matches)
//	99 challenges: active challenges.json entries with the player's progress
//
// The player = the console's oldest FFC profile, resolved through admin links (#53).

const ffcNewsTop = 5

func ffcNewsSeason(now time.Time) ffcstats.Season {
	for _, s := range ffcSeasons() {
		if !now.Before(s.Start) && (s.End.IsZero() || now.Before(s.End)) {
			return s
		}
	}
	return ffcstats.Season{}
}

func ffcNews(gamecd string, userID uint64) []ffcbonus.NewsItem {
	pids, err := db.ProfilesOfUser(userID, gamecd)
	if err != nil {
		logging.Error("API", "FFC news: profiles:", err)
		return nil
	}
	if len(pids) == 0 {
		return nil
	}
	links := ffcPlayerLinks()
	pid := ffcstats.Resolve(links, pids[0])
	now := ffcNow()
	s := ffcNewsSeason(now)
	ms, err := ffcLoadMatchesCached(s.Start, s.End)
	if err != nil {
		logging.Error("API", "FFC news: matches:", err)
		return nil
	}
	ms = ffcstats.FilterMode(ffcstats.ApplyLinks(ffcstats.Filter(ms, nil, s.Start, s.End, []string{"confirmed"}), links), "ranked")
	var cr []ffcstats.ChallengeResult
	if cs := ffcChallenges(); len(cs) > 0 {
		if cr, err = ffcChallengeResults(cs, now); err != nil {
			logging.Error("API", "FFC news: challenges:", err)
			cr = nil
		}
	}
	return ffcNewsItems(ms, s.ID, pid, cr)
}

// ffcNewsItems: pure part of ffcNews (tests).
func ffcNewsItems(ms []ffcstats.Match, season string, pid uint32, cr []ffcstats.ChallengeResult) []ffcbonus.NewsItem {
	var out []ffcbonus.NewsItem
	add := func(name, text string) {
		if text != "" {
			out = append(out, ffcbonus.NewsItem{Name: name, Body: []byte(text)})
		}
	}
	add("97", ffcstats.ProfileNews(ffcstats.Leaderboard(ms, 0), pid, season))
	add("98", ffcstats.TopNews(ffcstats.Leaderboard(ms, ffcNewsTop), ffcNewsTop))
	add("99", ffcstats.ChallengesNews(cr, pid))
	return out
}
