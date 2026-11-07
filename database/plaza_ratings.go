package database

// Per-rater view of one sake record's ratings (ffc-decomp ROADMAP #57, admin). Ratings live in sake_ratings
// (RateSakeRecord); GetSakeRecords already returns the count and sum per record.

type SakeRating struct {
	Rater  int32 `json:"rater_pid"`
	Rating int16 `json:"rating"`
}

func (c *Connection) GetSakeRecordRatings(gameId int, tableId string, recordId int32) ([]SakeRating, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT rater_profile_id, rating FROM sake_ratings
		WHERE game_id = $1 AND table_id = $2 AND record_id = $3 ORDER BY rater_profile_id`, gameId, tableId, recordId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SakeRating{}
	for rows.Next() {
		var s SakeRating
		if err := rows.Scan(&s.Rater, &s.Rating); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
