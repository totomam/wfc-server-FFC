package sake

// FFC local PvE (ffc-decomp ROADMAP #102 local variant, s111-online): when "Battle with Anyone" finds nobody, the
// modded ROM starts the stock Team Plaza battle against record id PVERecordID instead of a real record. The server
// answers GetSpecificRecords(PVERecordID) with a server-chosen team (a server-made team, owner 999000001: ghosts and
// #101 bosses, preferred; else any other player's team), its recordid rewritten to PVERecordID so the game rates the
// magic id afterwards. RateRecord(PVERecordID) stores nothing (no ranking, no ghost rating), logs the result and sends
// the webhook event "pve_result"; it answers AlreadyRated (FFC accepts it; NotRateable = error 41068, s101).

import (
	"math/rand"

	"wwfc/common"
	"wwfc/database"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

const (
	PVERecordID     = int32(0x7FFFFF00)
	pveServerOwner  = int32(999000001)
	pveGameName     = "dental2ds"
	pveRatingWinner = 100 // FFC rates 100 when the rater won, 200 when it lost (s101)
)

var pveRand = rand.Intn

func isPVERequest(gameInfo common.GameInfo, ids []int32) bool {
	return gameInfo.Name == pveGameName && len(ids) == 1 && ids[0] == PVERecordID
}

// pickPVERecord chooses the opponent: a random server-made team if any, else a random team not owned by the player.
func pickPVERecord(records []database.SakeRecord, profileId uint32) (database.SakeRecord, bool) {
	var server, others []database.SakeRecord
	for _, r := range records {
		switch {
		case r.OwnerId == pveServerOwner:
			server = append(server, r)
		case r.OwnerId != int32(profileId):
			others = append(others, r)
		}
	}
	pool := server
	if len(pool) == 0 {
		pool = others
	}
	if len(pool) == 0 {
		return database.SakeRecord{}, false
	}
	return pool[pveRand(len(pool))], true
}

func getPVERecord(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	fail := func(r Result) StorageResponseBody {
		return StorageResponseBody{GetSpecificRecordsResponse: &GetSpecificRecordsResponse{GetSpecificRecordsResult: r}}
	}
	all := request
	all.Filter = ""
	all.OwnerIDs.Int = nil
	records, table, result := readableRecords(moduleName, profileId, gameInfo, all, nil)
	if result != ResultSuccess {
		return fail(result)
	}
	rec, ok := pickPVERecord(records, profileId)
	if !ok {
		logging.Warn(moduleName, "PvE: no team to offer profile", aurora.Cyan(profileId))
		return fail(ResultSuccess) // empty: the game says "team couldn't be found" and returns to the menu
	}
	src := rec.RecordId
	rec.RecordId = PVERecordID
	values, result := fillResponseValues(moduleName, profileId, table, []database.SakeRecord{rec}, request)
	logging.Notice(moduleName, "PvE: profile", aurora.Cyan(profileId), "battles record", aurora.Cyan(src), "owner", aurora.Cyan(rec.OwnerId))
	return StorageResponseBody{GetSpecificRecordsResponse: &GetSpecificRecordsResponse{GetSpecificRecordsResult: result, Values: values}}
}

func ratePVERecord(moduleName string, profileId uint32, request StorageRequestCommon) StorageResponseBody {
	outcome := "lost"
	if request.Rating == pveRatingWinner {
		outcome = "won"
	}
	logging.Notice(moduleName, "PvE result: profile", aurora.Cyan(profileId), aurora.BrightGreen(outcome), "(rating", aurora.Cyan(request.Rating).String()+", not ranked)")
	logging.Event("pve_result", map[string]any{"Player ID": profileId, "Result": outcome, "Mode": "PvE (CPU, not ranked)"})
	return StorageResponseBody{RateRecordResponse: &RateRecordResponse{RateRecordResult: ResultAlreadyRated}}
}
