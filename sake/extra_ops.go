package sake

// Sake operations beyond Create/Update/GetMy/Search, plus the GameSpy "virtual" fields
// (update_time, num_ratings, average_rating, sum_ratings, my_rating) and generic sorting.
// Ratings are stored in the sake_ratings table. update_time and my_rating cannot be used in filters;
// average_rating compares as numeric but the filter language only has integer literals.
// Behaviour changes for every game: SearchForRecords honours offset and sorts by any field (ints, floats,
// strings, virtual fields), Max fields per record is 128, /pr honours wregion. LimitPerOwner is opt-in (EnforceLimit).
//
// Test status: Go unit tests (extra_ops_test.go, database/sake_filter_test.go) cover sorting, offset/max,
// the reported record limit, required fields and filter SQL generation; they need no database.
// NOT run: Mario Kart Wii (offset paging, 128-field cap, /pr wregion changes affect it and were never
// exercised against a real client or a MKW-shaped request); EnforceLimit's insert path and the rating
// SQL are only covered by the live dental2ds kit test (team_plaza_test.py), not by Go tests (they need
// PostgreSQL); the UpdateRecord NoPermission mapping (ErrSakeFieldLimitExceeded) was never executed.
// Needed by Fossil Fighters: Champions (dental2ds) Team Plaza; written generically.

import (
	"encoding/xml"
	"errors"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"wwfc/common"
	"wwfc/database"
	"wwfc/logging"

	"github.com/jackc/pgx/v4"
	"github.com/logrusorgru/aurora/v3"
)

type GetRandomRecordsResponse struct {
	GetRandomRecordsResult Result
	Values                 ArrayOfArrayOfRecordValue `xml:"values"`
}

type GetSpecificRecordsResponse struct {
	GetSpecificRecordsResult Result
	Values                   ArrayOfArrayOfRecordValue `xml:"values"`
}

type DeleteRecordResponse struct {
	DeleteRecordResult Result
}

type RateRecordResponse struct {
	RateRecordResult Result
	NumRatings       int32   `xml:"numRatings"`
	AverageRating    float32 `xml:"averageRating"`
}

type GetRecordCountResponse struct {
	GetRecordCountResult Result
	Count                int32 `xml:"count"`
}

type GetRecordLimitResponse struct {
	GetRecordLimitResult Result
	LimitPerOwner        int32 `xml:"limitPerOwner"`
	NumOwned             int32 `xml:"numOwned"`
}

// syntheticField returns the GameSpy "virtual" fields. Rating values come from the sake_ratings table
// (record.NumRatings/SumRatings, record.MyRating) and update_time from the record row; anything a
// client may have stored under these names is ignored.
func syntheticField(record database.SakeRecord, field string) (CommonValue, bool) {
	switch field {
	case "update_time":
		return CommonValue{XMLName: xml.Name{Local: "dateAndTimeValue"}, Value: record.UpdateTime.UTC().Format(DateAndTimeFormat)}, true
	case "num_ratings", "sum_ratings", "my_rating":
		v := record.NumRatings
		switch field {
		case "sum_ratings":
			v = record.SumRatings
		case "my_rating":
			v = record.MyRating
		}
		return CommonValue{XMLName: xml.Name{Local: "intValue"}, Value: strconv.FormatInt(v, 10)}, true
	case "average_rating":
		return CommonValue{XMLName: xml.Name{Local: "floatValue"}, Value: strconv.FormatFloat(averageRating(record.NumRatings, record.SumRatings), 'f', -1, 32)}, true
	}
	return CommonValue{}, false
}

func averageRating(num int64, sum int64) float64 {
	if num == 0 {
		return 0
	}
	return float64(sum) / float64(num)
}

func sortValue(record database.SakeRecord, field string) (string, bool) {
	switch field {
	case "ownerid":
		return strconv.Itoa(int(record.OwnerId)), true
	case "recordid":
		return strconv.Itoa(int(record.RecordId)), true
	}
	if v, ok := syntheticField(record, field); ok {
		return v.Value, true
	}
	f, ok := record.Fields[field]
	return f.Value, ok
}

// sortRecords sorts by a GameSpy sort expression: "field [ASC|DESC][, field2 [ASC|DESC]...]".
func sortRecords(records []database.SakeRecord, sortExpr string) {
	type key struct {
		field string
		desc  bool
	}
	var keys []key
	for _, part := range strings.Split(sortExpr, ",") {
		words := strings.Fields(part)
		if len(words) == 0 {
			continue
		}
		k := key{field: words[0]}
		if len(words) > 1 && strings.EqualFold(words[1], "desc") {
			k.desc = true
		}
		keys = append(keys, k)
	}
	// Total order: per key numeric values (ParseInt, else ParseFloat) before non-numeric ones,
	// non-numeric compared as strings, records without the field last; ties (and an empty sort
	// expression) fall back to the record ID so offset/max paging is deterministic.
	sort.SliceStable(records, func(l, r int) bool {
		for _, k := range keys {
			lv, lok := sortValue(records[l], k.field)
			rv, rok := sortValue(records[r], k.field)
			if !lok || !rok {
				if lok != rok {
					return lok // records that have the field first
				}
				continue
			}
			cmp := compareSortValues(lv, rv)
			if cmp == 0 {
				continue
			}
			if k.desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return records[l].RecordId < records[r].RecordId
	})
}

// sortNumber classifies a sort value: numeric if it parses as an int64 or as a finite float64
// ("inf", "nan" and out-of-range values like "1e400" are plain strings). Comparing numerics exactly
// (big.Float for int/float mixes) keeps the order transitive, which sort.SliceStable needs.
func sortNumber(v string) (i int64, f float64, isInt bool, ok bool) {
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, 0, true, true
	}
	if x, err := strconv.ParseFloat(v, 64); err == nil && !math.IsNaN(x) && !math.IsInf(x, 0) {
		return 0, x, false, true
	}
	return 0, 0, false, false
}

func compareSortValues(lv string, rv string) int {
	li, lf, lInt, lok := sortNumber(lv)
	ri, rf, rInt, rok := sortNumber(rv)
	switch {
	case lok && rok:
		switch {
		case lInt && rInt:
			switch {
			case li < ri:
				return -1
			case li > ri:
				return 1
			}
			return 0
		case !lInt && !rInt:
			switch {
			case lf < rf:
				return -1
			case lf > rf:
				return 1
			}
			return 0
		}
		var l, r big.Float
		if lInt {
			l.SetInt64(li)
		} else {
			l.SetFloat64(lf)
		}
		if rInt {
			r.SetInt64(ri)
		} else {
			r.SetFloat64(rf)
		}
		return l.Cmp(&r)
	case lok:
		return -1
	case rok:
		return 1
	}
	return strings.Compare(lv, rv)
}

func applyOffsetMax(records []database.SakeRecord, offset int32, max int32) []database.SakeRecord {
	if offset > 0 {
		if int(offset) >= len(records) {
			return nil
		}
		records = records[offset:]
	}
	if max > 0 && len(records) > int(max) {
		records = records[:max]
	}
	return records
}

func (body *StorageResponseBody) setExtraResultTag(xmlName string, result Result) {
	switch xmlName {
	case SakeNamespace + "/GetRandomRecords":
		body.GetRandomRecordsResponse = &GetRandomRecordsResponse{GetRandomRecordsResult: result}
	case SakeNamespace + "/GetSpecificRecords":
		body.GetSpecificRecordsResponse = &GetSpecificRecordsResponse{GetSpecificRecordsResult: result}
	case SakeNamespace + "/DeleteRecord":
		body.DeleteRecordResponse = &DeleteRecordResponse{DeleteRecordResult: result}
	case SakeNamespace + "/RateRecord":
		body.RateRecordResponse = &RateRecordResponse{RateRecordResult: result}
	case SakeNamespace + "/GetRecordCount":
		body.GetRecordCountResponse = &GetRecordCountResponse{GetRecordCountResult: result}
	case SakeNamespace + "/GetRecordLimit":
		body.GetRecordLimitResponse = &GetRecordLimitResponse{GetRecordLimitResult: result}
	}
}

// readableRecords fetches records a profile may read from a non-reserved table.
func readableRecords(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon, recordIds []int32) ([]database.SakeRecord, *SakeTable, Result) {
	if request.TableID == "" {
		return nil, nil, ResultTableNotFound
	}
	// Undefined tables are TableNotFound for every extra operation (as Create/Rate/GetRecordLimit);
	// SearchForRecords in storage.go keeps upstream's behaviour for undefined tables.
	table := GetTable(gameInfo.Name, request.TableID)
	if table == nil || table.Reserved {
		return nil, nil, ResultTableNotFound
	}
	ownerIds := request.OwnerIDs.Int
	if !table.AllowsPublicRead() {
		ownerIds = []int32{int32(profileId)}
	}
	records, err := db.GetSakeRecords(gameInfo.GameID, ownerIds, request.TableID, recordIds, request.Fields.String, request.Filter)
	if err != nil {
		logging.Error(moduleName, "Failed to get sake records from the database:", err)
		if errors.Is(err, database.ErrSakeFilterUnsupported) {
			return nil, table, ResultFilterInvalid
		}
		return nil, table, ResultDatabaseUnavailable
	}
	return records, table, ResultSuccess
}

// Known limitation: GetRandomRecords and GetRecordCount load every matching row (readableRecords) and
// shuffle/count in Go; fine for a table of a few thousand teams, not tuned for very large tables.
func getRandomRecords(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	records, table, result := readableRecords(moduleName, profileId, gameInfo, request, nil)
	if result != ResultSuccess {
		return StorageResponseBody{GetRandomRecordsResponse: &GetRandomRecordsResponse{GetRandomRecordsResult: result}}
	}
	rand.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
	// GameSpy's GetRandomRecords has no offset parameter
	records = applyOffsetMax(records, 0, request.Max)
	values, result := fillResponseValues(moduleName, profileId, table, records, request)
	logging.Info(moduleName, "Returning", aurora.Cyan(len(records)), "random records from table", aurora.Cyan(request.TableID), "with filter", aurora.Cyan(request.Filter))
	return StorageResponseBody{GetRandomRecordsResponse: &GetRandomRecordsResponse{GetRandomRecordsResult: result, Values: values}}
}

func getSpecificRecords(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	if len(request.RecordIDs.Int) == 0 {
		return StorageResponseBody{GetSpecificRecordsResponse: &GetSpecificRecordsResponse{GetSpecificRecordsResult: ResultRecordNotFound}}
	}
	records, table, result := readableRecords(moduleName, profileId, gameInfo, request, request.RecordIDs.Int)
	if result != ResultSuccess {
		return StorageResponseBody{GetSpecificRecordsResponse: &GetSpecificRecordsResponse{GetSpecificRecordsResult: result}}
	}
	// Keep the order the client asked for
	pos := map[int32]int{}
	for i, id := range request.RecordIDs.Int {
		pos[id] = i
	}
	sort.SliceStable(records, func(l, r int) bool { return pos[records[l].RecordId] < pos[records[r].RecordId] })
	values, result := fillResponseValues(moduleName, profileId, table, records, request)
	logging.Info(moduleName, "Returning", aurora.Cyan(len(records)), "specific records from table", aurora.Cyan(request.TableID))
	return StorageResponseBody{GetSpecificRecordsResponse: &GetSpecificRecordsResponse{GetSpecificRecordsResult: result, Values: values}}
}

func deleteRecord(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	resp := func(r Result) StorageResponseBody {
		return StorageResponseBody{DeleteRecordResponse: &DeleteRecordResponse{DeleteRecordResult: r}}
	}
	if request.TableID == "" {
		return resp(ResultTableNotFound)
	}
	table := GetTable(gameInfo.Name, request.TableID)
	if table == nil {
		return resp(ResultTableNotFound)
	}
	if table.Reserved {
		return resp(ResultNoPermission)
	}
	if !table.AllowsOwnerDelete() {
		return resp(ResultNoPermission)
	}
	if err := db.DeleteSakeRecord(gameInfo.GameID, request.TableID, request.RecordID, int32(profileId)); err != nil {
		if err == database.ErrSakeNotOwned {
			logging.Error(moduleName, "Delete of record", aurora.Cyan(request.RecordID), "not owned by / not found for profile", aurora.Cyan(profileId))
			return resp(ResultRecordNotFound)
		}
		logging.Error(moduleName, "Failed to delete sake record:", err)
		return resp(ResultDatabaseUnavailable)
	}
	logging.Info(moduleName, "Deleted record", aurora.Cyan(request.RecordID), "in table", aurora.Cyan(request.TableID), "for profile", aurora.Cyan(profileId))
	return resp(ResultSuccess)
}

func rateRecord(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	resp := func(r Result, num int64, sum int64) StorageResponseBody {
		return StorageResponseBody{RateRecordResponse: &RateRecordResponse{RateRecordResult: r, NumRatings: int32(num), AverageRating: float32(averageRating(num, sum))}}
	}
	if request.TableID == "" {
		return resp(ResultTableNotFound, 0, 0)
	}
	// Only tables that are defined, explicitly rateable and publicly readable can be rated
	// (anything else would let players write rating rows into records they cannot even read).
	table := GetTable(gameInfo.Name, request.TableID)
	if table == nil || table.Reserved {
		return resp(ResultTableNotFound, 0, 0)
	}
	if table.Rateable != RateableYes {
		return resp(ResultNotRateable, 0, 0)
	}
	if !table.AllowsPublicRead() {
		return resp(ResultNoPermission, 0, 0)
	}
	num, sum, err := db.RateSakeRecord(gameInfo.GameID, request.TableID, request.RecordID, int32(profileId), request.Rating)
	switch err {
	case nil:
	case pgx.ErrNoRows:
		return resp(ResultRecordNotFound, 0, 0)
	case database.ErrSakeOwnRecord:
		// There is no result code for "own record" in the SAKE protocol. FFC (dental2ds) shows
		// "Communication error 41068" for NotRateable after battling its own Team Plaza team, but accepts
		// AlreadyRated (FFC decomp s101-online, melonDS), so answer that: nothing is stored either way.
		return resp(ResultAlreadyRated, num, sum)
	case database.ErrSakeAlreadyRated:
		return resp(ResultAlreadyRated, num, sum)
	default:
		logging.Error(moduleName, "Failed to store rating:", err)
		return resp(ResultDatabaseUnavailable, 0, 0)
	}
	logging.Info(moduleName, "Profile", aurora.Cyan(profileId), "rated record", aurora.Cyan(request.RecordID), "with", aurora.Cyan(request.Rating))
	return resp(ResultSuccess, num, sum)
}

func getRecordCount(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	records, _, result := readableRecords(moduleName, profileId, gameInfo, request, nil)
	return StorageResponseBody{GetRecordCountResponse: &GetRecordCountResponse{GetRecordCountResult: result, Count: int32(len(records))}}
}

// getRecordLimit reports the limit CreateRecord really enforces: the table's LimitPerOwner when the
// table sets EnforceLimit, otherwise the global per-profile cap (MaxSakeRecordsPerProfile).
func getRecordLimit(moduleName string, profileId uint32, gameInfo common.GameInfo, request StorageRequestCommon) StorageResponseBody {
	fail := func(r Result) StorageResponseBody {
		return StorageResponseBody{GetRecordLimitResponse: &GetRecordLimitResponse{GetRecordLimitResult: r}}
	}
	if request.TableID == "" {
		return fail(ResultTableNotFound)
	}
	table := GetTable(gameInfo.Name, request.TableID)
	if table == nil || table.Reserved {
		return fail(ResultTableNotFound)
	}
	limit := recordLimitFor(table)
	n, err := db.CountSakeRecordsOwned(gameInfo.GameID, request.TableID, int32(profileId))
	if err != nil {
		return fail(ResultDatabaseUnavailable)
	}
	return StorageResponseBody{GetRecordLimitResponse: &GetRecordLimitResponse{GetRecordLimitResult: ResultSuccess, LimitPerOwner: int32(limit), NumOwned: int32(n)}}
}

func recordLimitFor(table *SakeTable) int {
	if table != nil && table.EnforceLimit && table.LimitPerOwner > 0 {
		return table.LimitPerOwner
	}
	return MaxSakeRecordsPerProfile
}
