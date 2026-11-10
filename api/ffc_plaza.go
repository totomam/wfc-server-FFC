package api

import (
	"net/http"
	"sort"
	"strconv"
	"wwfc/database"
)

// FFC Team Plaza team / ghost ratings (ffc-decomp ROADMAP #57). Ratings come from the game's RateRecord
// (sake_ratings, one per rater); battle_* are the owner's self-reported Plaza results (unverified).
//   GET /api/ffc/plaza[?sort=rating|ratings|wins&min=N&ghosts=1&limit=N]   public: teams visible right now (event
//       windows applied), average + count of ratings, no rater ids.
//   GET /api/ffc/admin/ratings?record_id=N   one record: every rater pid + rating, histogram.

const ffcGhostOwner = 999000001 // decomp tools/online_ghost.py GHOST_OWNER: server-made teams, never a real player

type FFCPlazaTeam struct {
	RecordID   int32   `json:"record_id"`
	OwnerID    int32   `json:"owner_pid"`
	Ghost      bool    `json:"ghost"`
	TeamName   string  `json:"team_name"`
	PlayerName string  `json:"player_name"`
	Ratings    int64   `json:"ratings"`
	AvgRating  float64 `json:"avg_rating"`
	Wins       int     `json:"battle_win"`
	Losses     int     `json:"battle_lose"`
	Draws      int     `json:"battle_draw"`
}

func ffcPlazaTeams(recs []database.SakeRecord, sortBy string, min int64, ghostsOnly bool, limit int) []FFCPlazaTeam {
	out := []FFCPlazaTeam{}
	for _, r := range recs {
		ghost := r.OwnerId == ffcGhostOwner
		if r.NumRatings < min || (ghostsOnly && !ghost) {
			continue
		}
		t := FFCPlazaTeam{RecordID: r.RecordId, OwnerID: r.OwnerId, Ghost: ghost, Ratings: r.NumRatings,
			TeamName: r.Fields["team_name"].Value, PlayerName: r.Fields["player_name"].Value}
		if r.NumRatings > 0 {
			t.AvgRating = float64(r.SumRatings) / float64(r.NumRatings)
		}
		t.Wins, _ = strconv.Atoi(r.Fields["battle_win"].Value)
		t.Losses, _ = strconv.Atoi(r.Fields["battle_lose"].Value)
		t.Draws, _ = strconv.Atoi(r.Fields["battle_draw"].Value)
		out = append(out, t)
	}
	key := func(t FFCPlazaTeam) (float64, float64) {
		switch sortBy {
		case "ratings":
			return float64(t.Ratings), t.AvgRating
		case "wins":
			return float64(t.Wins), float64(-t.Losses)
		}
		return t.AvgRating, float64(t.Ratings)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a1, a2 := key(out[i])
		b1, b2 := key(out[j])
		if a1 != b1 {
			return a1 > b1
		}
		if a2 != b2 {
			return a2 > b2
		}
		return out[i].RecordID < out[j].RecordID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func HandleFFCPlaza(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleNone)
	if err != nil {
		return
	}
	recs, err := db.GetSakeRecords(ffcSakeGameID, nil, ffcSakeTable, nil, nil, "")
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	min, _ := strconv.ParseInt(q.Get("min"), 10, 64)
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 500 {
		limit = 100
	}
	teams := ffcPlazaTeams(recs, q.Get("sort"), min, q.Get("ghosts") == "1", limit)
	replyOK(w, map[string]any{"total": len(recs), "teams": teams})
}

func HandleFFCRatings(w http.ResponseWriter, r *http.Request) {
	q, err := parseGet(r, w, RoleModerator)
	if err != nil {
		return
	}
	id, err := strconv.ParseInt(q.Get("record_id"), 10, 32)
	if err != nil || id == 0 {
		replyError(w, http.StatusBadRequest, "invalid_record_id")
		return
	}
	rs, err := db.GetSakeRecordRatings(ffcSakeGameID, ffcSakeTable, int32(id))
	if err != nil {
		replyError(w, http.StatusInternalServerError, "database_error")
		return
	}
	hist := map[int16]int{}
	sum := 0
	for _, x := range rs {
		hist[x.Rating]++
		sum += int(x.Rating)
	}
	avg := 0.0
	if len(rs) > 0 {
		avg = float64(sum) / float64(len(rs))
	}
	replyOK(w, map[string]any{"record_id": id, "ratings": len(rs), "avg_rating": avg, "histogram": hist, "raters": rs})
}
