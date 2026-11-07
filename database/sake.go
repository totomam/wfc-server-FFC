package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"wwfc/common"
	"wwfc/filter"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	SakeFieldTypeByte          = 0
	SakeFieldTypeShort         = 1
	SakeFieldTypeInt           = 2
	SakeFieldTypeFloat         = 3
	SakeFieldTypeAsciiString   = 4
	SakeFieldTypeUnicodeString = 5
	SakeFieldTypeBoolean       = 6
	SakeFieldTypeDateAndTime   = 7
	SakeFieldTypeBinaryData    = 8
	SakeFieldTypeInt64         = 9
)

type SakeFieldType int

type SakeField struct {
	Type  SakeFieldType `json:"type"`
	Value string        `json:"value"`
}

type SakeRecord struct {
	GameId   int
	OwnerId  int32
	TableId  string
	RecordId int32
	Fields   map[string]SakeField
	// Last create/update time (read only; filled by GetSakeRecords)
	UpdateTime time.Time
	// Rating aggregates from sake_ratings (read only; filled by GetSakeRecords)
	NumRatings int64
	SumRatings int64
	// The requesting profile's own rating, only set by GetSakeMyRatings
	MyRating int64
}

const (
	getSakeRecordsQuery = `
		SELECT owner_id, record_id, fields, COALESCE(update_time, now() AT TIME ZONE 'UTC'), ratings.num, ratings.sum
		FROM sake_records
		CROSS JOIN LATERAL (
			SELECT COUNT(*) AS num, COALESCE(SUM(sr.rating), 0) AS sum FROM sake_ratings sr
			WHERE sr.game_id = sake_records.game_id AND sr.table_id = sake_records.table_id AND sr.record_id = sake_records.record_id
		) ratings
		WHERE game_id = $1 
		  AND table_id = $2 
		  AND (cardinality($4::integer[]) = 0 OR record_id = ANY($4::integer[])) 
		  AND (cardinality($3::integer[]) = 0 OR owner_id = ANY($3::integer[]))` + sakeEventVisible

	updateSakeRecordQuery = `
        UPDATE sake_records 
        SET 
            fields = CASE WHEN owner_id = $4 THEN fields || $5 ELSE fields END, 
            update_time = CASE WHEN owner_id = $4 THEN now() AT TIME ZONE 'UTC' ELSE update_time END
        WHERE game_id = $1 
          AND table_id = $2 
          AND record_id = $3 
        RETURNING owner_id`

	insertSakeRecordQuery = `
		INSERT INTO sake_records (game_id, table_id, owner_id, fields) 
		VALUES ($1, $2, $3, $4)
		RETURNING record_id`

	deleteSakeRecordQuery = `
		DELETE FROM sake_records 
		WHERE game_id = $1 
		  AND table_id = $2 
		  AND record_id = $3 
		  AND owner_id = $4`

	checkMaxSakeRecordsQuery = `
		SELECT COUNT(*) 
		FROM sake_records 
		WHERE owner_id = $1`
)

var (
	ErrSakeNotOwned           = errors.New("record is not owned by the specified owner ID")
	ErrSakeFieldLimitExceeded = errors.New("record has too many fields")
	ErrSakeFilterUnsupported  = errors.New("filter is malformed or uses a field/operator that cannot be filtered on")
	ErrSakeRecordLimit        = errors.New("owner has reached the record limit of this table")
	ErrSakeAlreadyRated       = errors.New("profile has already rated this record")
	ErrSakeOwnRecord          = errors.New("a profile cannot rate its own record")
)

// DeleteSakeRecord deletes a record owned by ownerId. Returns ErrSakeNotOwned if no such record owned by ownerId exists.
func (c *Connection) DeleteSakeRecord(gameId int, tableId string, recordId int32, ownerId int32) error {
	tag, err := c.pool.Exec(c.ctx, deleteSakeRecordQuery, gameId, tableId, recordId, ownerId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSakeNotOwned
	}
	return nil
}

// RateSakeRecord stores a rating atomically: the (record, rater) primary key of sake_ratings makes
// concurrent duplicate ratings from one profile collapse into a single row.
// Returns pgx.ErrNoRows if the record does not exist, ErrSakeOwnRecord or ErrSakeAlreadyRated.
func (c *Connection) RateSakeRecord(gameId int, tableId string, recordId int32, raterId int32, rating byte) (numRatings int64, sumRatings int64, err error) {
	var ownerId int32
	err = c.pool.QueryRow(c.ctx, `SELECT owner_id FROM sake_records WHERE game_id = $1 AND table_id = $2 AND record_id = $3`,
		gameId, tableId, recordId).Scan(&ownerId)
	if err != nil {
		return 0, 0, err
	}
	if ownerId == raterId {
		return 0, 0, ErrSakeOwnRecord
	}

	tag, err := c.pool.Exec(c.ctx, `INSERT INTO sake_ratings (game_id, table_id, record_id, rater_profile_id, rating) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		gameId, tableId, recordId, raterId, int16(rating))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
			// The record was deleted in the meantime
			return 0, 0, pgx.ErrNoRows
		}
		return 0, 0, err
	}

	err = c.pool.QueryRow(c.ctx, `SELECT COUNT(*), COALESCE(SUM(rating), 0) FROM sake_ratings WHERE game_id = $1 AND table_id = $2 AND record_id = $3`,
		gameId, tableId, recordId).Scan(&numRatings, &sumRatings)
	if err != nil {
		return 0, 0, err
	}
	if tag.RowsAffected() == 0 {
		return numRatings, sumRatings, ErrSakeAlreadyRated
	}
	return numRatings, sumRatings, nil
}

// GetSakeMyRatings returns the ratings a profile gave to the given records (record ID -> rating).
func (c *Connection) GetSakeMyRatings(gameId int, tableId string, raterId int32, recordIds []int32) (map[int32]int64, error) {
	rows, err := c.pool.Query(c.ctx, `SELECT record_id, rating FROM sake_ratings WHERE game_id = $1 AND table_id = $2 AND rater_profile_id = $3 AND record_id = ANY($4::integer[])`,
		gameId, tableId, raterId, recordIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ratings := map[int32]int64{}
	for rows.Next() {
		var recordId int32
		var rating int64
		if err := rows.Scan(&recordId, &rating); err != nil {
			return nil, err
		}
		ratings[recordId] = rating
	}
	return ratings, rows.Err()
}

// CountSakeRecordsOwned returns how many records of a table a profile owns.
func (c *Connection) CountSakeRecordsOwned(gameId int, tableId string, ownerId int32) (int, error) {
	var n int
	err := c.pool.QueryRow(c.ctx, `SELECT COUNT(*) FROM sake_records WHERE game_id = $1 AND table_id = $2 AND owner_id = $3`,
		gameId, tableId, ownerId).Scan(&n)
	return n, err
}

// InsertSakeRecordLimited inserts a record unless its owner already has limit records in the table
// (ErrSakeRecordLimit). The count and insert run under a per-owner advisory lock so concurrent
// creates cannot exceed the limit.
func (c *Connection) InsertSakeRecordLimited(record SakeRecord, limit int) (recordId int32, err error) {
	fieldsJson, err := json.Marshal(record.Fields)
	if err != nil {
		return 0, err
	}

	for i := 0; i < 10; i++ {
		recordId, err = c.insertSakeRecordLimitedOnce(record, fieldsJson, limit)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.UniqueViolation {
			break
		}
		// Retry if unique violation occurred, as the record ID is generated randomly
	}
	return recordId, err
}

func (c *Connection) insertSakeRecordLimitedOnce(record SakeRecord, fieldsJson []byte, limit int) (recordId int32, err error) {
	tx, err := c.pool.Begin(c.ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(c.ctx) }()

	_, err = tx.Exec(c.ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		strconv.Itoa(record.GameId)+"/"+record.TableId+"/"+strconv.Itoa(int(record.OwnerId)))
	if err != nil {
		return 0, err
	}

	var count int
	err = tx.QueryRow(c.ctx, `SELECT COUNT(*) FROM sake_records WHERE game_id = $1 AND table_id = $2 AND owner_id = $3`,
		record.GameId, record.TableId, record.OwnerId).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count >= limit {
		return 0, ErrSakeRecordLimit
	}

	err = tx.QueryRow(c.ctx, insertSakeRecordQuery, record.GameId, record.TableId, record.OwnerId, fieldsJson).Scan(&recordId)
	if err != nil {
		return 0, err
	}
	return recordId, tx.Commit(c.ctx)
}

func parseSakeFieldsFromJson(fieldsJson []byte) (map[string]SakeField, error) {
	var fields map[string]SakeField
	err := json.Unmarshal(fieldsJson, &fields)
	if err != nil {
		return nil, err
	}

	return fields, nil
}

func (c *Connection) GetSakeRecords(gameId int, ownerIds []int32, tableId string, recordIds []int32, fields []string, filterExpr string) ([]SakeRecord, error) {
	if fields == nil {
		fields = []string{}
	}
	common.MaybeUnused(fields)
	if ownerIds == nil {
		ownerIds = []int32{}
	}
	if recordIds == nil {
		recordIds = []int32{}
	}

	query := getSakeRecordsQuery
	if filterExpr != "" {
		tree, err := filter.Parse(filterExpr)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSakeFilterUnsupported, err)
		}

		var filterQuery string
		err = c.pool.AcquireFunc(c.ctx, func(conn *pgxpool.Conn) error {
			filterQuery, err = createSqlFilter(conn.Conn().PgConn(), tree)
			return err
		})
		if err != nil {
			return nil, err
		}

		// This filter has been entirely rewritten by our filter code,
		// based on the expression supplied by the user. This should be safe!!!
		query += " AND (" + filterQuery + ")"
	}

	rows, err := c.pool.Query(c.ctx, query, gameId, tableId, ownerIds, recordIds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SakeRecord
	for rows.Next() {
		record := SakeRecord{
			GameId:  gameId,
			TableId: tableId,
		}
		var fieldsJson []byte
		if err := rows.Scan(&record.OwnerId, &record.RecordId, &fieldsJson, &record.UpdateTime, &record.NumRatings, &record.SumRatings); err != nil {
			return nil, err
		}
		fields, err := parseSakeFieldsFromJson(fieldsJson)
		if err != nil {
			return nil, err
		}
		record.Fields = fields

		records = append(records, record)
	}

	return records, nil
}

func (c *Connection) UpdateSakeRecord(record SakeRecord, ownerId int32) error {
	fieldsJson, err := json.Marshal(record.Fields)
	if err != nil {
		return err
	}
	var existingOwnerId int32
	err = c.pool.QueryRow(c.ctx, updateSakeRecordQuery, record.GameId, record.TableId, record.RecordId, ownerId, fieldsJson).Scan(&existingOwnerId)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.CheckViolation {
			return ErrSakeFieldLimitExceeded
		}
		return err
	}
	if ownerId != 0 && existingOwnerId != ownerId {
		return ErrSakeNotOwned
	}

	return nil
}

func (c *Connection) InsertSakeRecord(record SakeRecord) (recordId int32, err error) {
	fieldsJson, err := json.Marshal(record.Fields)
	if err != nil {
		return 0, err
	}

	for i := 0; i < 10; i++ {
		err = c.pool.QueryRow(c.ctx, insertSakeRecordQuery, record.GameId, record.TableId, record.OwnerId, fieldsJson).Scan(&recordId)
		if err == nil {
			break
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			break
		}
		if pgErr.Code != pgerrcode.UniqueViolation {
			break
		}
		// Retry if unique violation occurred, as the record ID is generated randomly
	}
	return recordId, err
}

func (c *Connection) IsMaxSakeRecordsReached(profileId uint32, maxRecords int) (bool, error) {
	var count int
	err := c.pool.QueryRow(c.ctx, checkMaxSakeRecordsQuery, profileId).Scan(&count)
	if err != nil {
		return false, err
	}
	return count >= maxRecords, nil
}
