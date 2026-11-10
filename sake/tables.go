package sake

import (
	"encoding/base64"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
	"wwfc/database"
)

const (
	MaxSakeRecordsPerProfile = 96
	MaxSakeFieldsPerRecord   = 128 // (all games; upstream: 64) dental2ds/tbl_main_na stores ~85 fields
	MaxSakeFieldValueLength  = 4096
)

const DateAndTimeFormat = "2006-01-02T15:04:05.000"

const (
	RateableUnknown = iota
	RateableYes
	RateableNo
)

type Rateable byte

const (
	OwnerTypeProfile = iota
	OwnerTypeBackend
)

type OwnerType byte

const (
	PermissionDefault = iota
	PermissionAllowed
	PermissionDenied
)

type Permission byte

type SakeFieldDefinition struct {
	// Type of the field.
	Type database.SakeFieldType
	// If empty, no default value will be set.
	// "{EMPTY}" can be used to set an empty string default value.
	// "{CURRENT_TIMESTAMP}" can be used to set the current timestamp as the default value (only for DateAndTime type).
	Default string
	// If zero, the default field length limit will be used.
	// For UnicodeString/AsciiString fields it counts characters (runes) after decoding, else bytes.
	LengthLimit int

	// Optional function for custom validation.
	IsValidFunc func(value string) bool
	// Optional function for custom filtering. This function receives the value from the client AFTER validation, before inserting into the database.
	FilterFromClientFunc func(value string, isOwner bool) (string, Result)
	// Optional function for custom filtering. This function receives the value from the database before sending to the client.
	FilterFromDatabaseFunc func(value string, isOwner bool) (string, Result)
}

type SakeTable struct {
	// Determines whether the 'average_rating', 'my_rating', 'num_ratings', 'sum_ratings' fields are automatically added.
	Rateable Rateable
	// Defaults to profile-owned records if not specified.
	OwnerType OwnerType
	// Defaults to allowed if OwnerType is OwnerTypeProfile, denied otherwise.
	PublicPermCreate Permission
	// Defaults to allowed.
	PublicPermRead Permission
	// Defaults to allowed.
	OwnerPermUpdate Permission
	// Defaults to allowed.
	OwnerPermDelete Permission
	// Override the default maximum number of records per owner.
	LimitPerOwner int
	// EnforceLimit makes CreateRecord refuse a new record once the owner has LimitPerOwner records
	// (opt-in per table: upstream never enforced LimitPerOwner, so existing games are unchanged).
	EnforceLimit bool
	// If true, fields not specified in this table definition will be rejected.
	Hardened bool
	// If true, CreateRecord is refused (FieldNotFound) unless the request carries every field of
	// Fields that has no Default, so partial records never become visible to searches.
	RequireAllFields bool
	// If true, Sake will return a NoPermission error for requests that don't have a custom handler
	Reserved bool
	// Custom handler for SearchForRecords. Returns an array of response Sake records.
	SearchForRecordsHandler func(string, StorageRequestCommon) ([]database.SakeRecord, bool)
	// Field definitions for this table. The key is the field name.
	Fields map[string]SakeFieldDefinition
}

// isFFCAiPolicy: dental2ds ai_1..ai_4 = the team's battle AI policy per situation (FFC decomp s101-online,
// docs/online/bosses/README.md). The opponent's console uses value+1 as a row of its 17-row tactics table
// (func_ov001_0218cab0 -> func_ov002_021c95bc), so an out-of-range value from a modified client would make OTHER
// players' consoles read past the table. The in-game picker offers 6 policies (0..5); server-made boss records
// (written to the database directly, tools/online_ghost.py boss) may use 0..16.
func isFFCAiPolicy(value string) bool {
	v, err := strconv.Atoi(value)
	return err == nil && v >= 0 && v <= 5
}

// isFFCGroup: dental2ds group1/group2 index 4 tables of 9 entries in the game (ov007, FFC decomp NET-05/NET-39),
// so only 0..8 is safe to serve to other players.
func isFFCGroup(value string) bool {
	v, err := strconv.Atoi(value)
	return err == nil && v >= 0 && v <= 8
}

// dental2ds tbl_main_na string caps (FFC decomp NET-39; client buffers: player_name 9, team_name 21, nicknames 16+NUL
// halfwords). team_name: ffc_badge.go serves "<badge> <name>" cut to FFCBadgeMaxLen (12), so the stored name may use
// the whole client room.
const (
	ffcPlayerNameMax = 8
	ffcTeamNameMax   = 20
	ffcNicknameMax   = 16
)

var TableDefinitions = map[string]SakeTable{
	// Fossil Fighters: Champions (dental2ds) Team Plaza: one team record per profile (EnforceLimit makes CreateRecord enforce LimitPerOwner), public search/rate (FFC decomp docs/online/TEAM_PLAZA.md).
	"dental2ds/tbl_main_na": {
		Rateable:         RateableYes,
		OwnerType:        OwnerTypeProfile,
		PublicPermCreate: PermissionAllowed,
		PublicPermRead:   PermissionAllowed,
		OwnerPermUpdate:  PermissionAllowed,
		OwnerPermDelete:  PermissionAllowed,
		LimitPerOwner:    1,
		Hardened:         true,
		EnforceLimit:     true,
		RequireAllFields: true,
		// The 82 fields CreateRecord writes (names and types from the game's ROM tables, FFC decomp
		// docs/online/wiimmfi/tbl_main_na.json); Hardened rejects anything else so a client cannot add
		// fields or change a field's type. battle_* are never sent by CreateRecord but GetSpecificRecords asks for them
		// (Battle button). Note battle_win/lose/draw are self-reported by the owner (no server-side verification).
		Fields: map[string]SakeFieldDefinition{
			"player_level":  {Type: database.SakeFieldTypeInt},
			"player_symbol": {Type: database.SakeFieldTypeInt},
			"team_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcTeamNameMax},
			"formation":     {Type: database.SakeFieldTypeInt},
			"ai_1":          {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCAiPolicy},
			"ai_2":          {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCAiPolicy},
			"ai_3":          {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCAiPolicy},
			"ai_4":          {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCAiPolicy},
			"team_rank":     {Type: database.SakeFieldTypeInt},
			"group1":        {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCGroup},
			"group2":        {Type: database.SakeFieldTypeInt, IsValidFunc: isFFCGroup},
			"player_name":   {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcPlayerNameMax},
			"rev_id_1":      {Type: database.SakeFieldTypeInt},
			"rev_id_2":      {Type: database.SakeFieldTypeInt},
			"rev_id_3":      {Type: database.SakeFieldTypeInt},
			"rev_id_4":      {Type: database.SakeFieldTypeInt},
			"rev_id_5":      {Type: database.SakeFieldTypeInt},
			"rev1_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcNicknameMax},
			"rev1_rank":     {Type: database.SakeFieldTypeInt},
			"rev1_bp":       {Type: database.SakeFieldTypeInt},
			"rev1_head_id":  {Type: database.SakeFieldTypeInt},
			"rev1_head_pt":  {Type: database.SakeFieldTypeInt},
			"rev1_body_id":  {Type: database.SakeFieldTypeInt},
			"rev1_body_pt":  {Type: database.SakeFieldTypeInt},
			"rev1_arm_id":   {Type: database.SakeFieldTypeInt},
			"rev1_arm_pt":   {Type: database.SakeFieldTypeInt},
			"rev1_leg_id":   {Type: database.SakeFieldTypeInt},
			"rev1_leg_pt":   {Type: database.SakeFieldTypeInt},
			"rev1_after_id": {Type: database.SakeFieldTypeInt},
			"rev1_after_pt": {Type: database.SakeFieldTypeInt},
			"rev2_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcNicknameMax},
			"rev2_rank":     {Type: database.SakeFieldTypeInt},
			"rev2_bp":       {Type: database.SakeFieldTypeInt},
			"rev2_head_id":  {Type: database.SakeFieldTypeInt},
			"rev2_head_pt":  {Type: database.SakeFieldTypeInt},
			"rev2_body_id":  {Type: database.SakeFieldTypeInt},
			"rev2_body_pt":  {Type: database.SakeFieldTypeInt},
			"rev2_arm_id":   {Type: database.SakeFieldTypeInt},
			"rev2_arm_pt":   {Type: database.SakeFieldTypeInt},
			"rev2_leg_id":   {Type: database.SakeFieldTypeInt},
			"rev2_leg_pt":   {Type: database.SakeFieldTypeInt},
			"rev2_after_id": {Type: database.SakeFieldTypeInt},
			"rev2_after_pt": {Type: database.SakeFieldTypeInt},
			"rev3_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcNicknameMax},
			"rev3_rank":     {Type: database.SakeFieldTypeInt},
			"rev3_bp":       {Type: database.SakeFieldTypeInt},
			"rev3_head_id":  {Type: database.SakeFieldTypeInt},
			"rev3_head_pt":  {Type: database.SakeFieldTypeInt},
			"rev3_body_id":  {Type: database.SakeFieldTypeInt},
			"rev3_body_pt":  {Type: database.SakeFieldTypeInt},
			"rev3_arm_id":   {Type: database.SakeFieldTypeInt},
			"rev3_arm_pt":   {Type: database.SakeFieldTypeInt},
			"rev3_leg_id":   {Type: database.SakeFieldTypeInt},
			"rev3_leg_pt":   {Type: database.SakeFieldTypeInt},
			"rev3_after_id": {Type: database.SakeFieldTypeInt},
			"rev3_after_pt": {Type: database.SakeFieldTypeInt},
			"rev4_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcNicknameMax},
			"rev4_rank":     {Type: database.SakeFieldTypeInt},
			"rev4_bp":       {Type: database.SakeFieldTypeInt},
			"rev4_head_id":  {Type: database.SakeFieldTypeInt},
			"rev4_head_pt":  {Type: database.SakeFieldTypeInt},
			"rev4_body_id":  {Type: database.SakeFieldTypeInt},
			"rev4_body_pt":  {Type: database.SakeFieldTypeInt},
			"rev4_arm_id":   {Type: database.SakeFieldTypeInt},
			"rev4_arm_pt":   {Type: database.SakeFieldTypeInt},
			"rev4_leg_id":   {Type: database.SakeFieldTypeInt},
			"rev4_leg_pt":   {Type: database.SakeFieldTypeInt},
			"rev4_after_id": {Type: database.SakeFieldTypeInt},
			"rev4_after_pt": {Type: database.SakeFieldTypeInt},
			"rev5_name":     {Type: database.SakeFieldTypeUnicodeString, LengthLimit: ffcNicknameMax},
			"rev5_rank":     {Type: database.SakeFieldTypeInt},
			"rev5_bp":       {Type: database.SakeFieldTypeInt},
			"rev5_head_id":  {Type: database.SakeFieldTypeInt},
			"rev5_head_pt":  {Type: database.SakeFieldTypeInt},
			"rev5_body_id":  {Type: database.SakeFieldTypeInt},
			"rev5_body_pt":  {Type: database.SakeFieldTypeInt},
			"rev5_arm_id":   {Type: database.SakeFieldTypeInt},
			"rev5_arm_pt":   {Type: database.SakeFieldTypeInt},
			"rev5_leg_id":   {Type: database.SakeFieldTypeInt},
			"rev5_leg_pt":   {Type: database.SakeFieldTypeInt},
			"rev5_after_id": {Type: database.SakeFieldTypeInt},
			"rev5_after_pt": {Type: database.SakeFieldTypeInt},
			"battle_win":    {Type: database.SakeFieldTypeInt, Default: "0"},
			"battle_lose":   {Type: database.SakeFieldTypeInt, Default: "0"},
			"battle_draw":   {Type: database.SakeFieldTypeInt, Default: "0"},
		},
	},

	"micchannelwii/userinfo": {
		Rateable:         RateableYes,
		OwnerType:        OwnerTypeProfile,
		PublicPermCreate: PermissionAllowed,
		PublicPermRead:   PermissionAllowed,
		OwnerPermUpdate:  PermissionAllowed,
		OwnerPermDelete:  PermissionAllowed,
		Fields: map[string]SakeFieldDefinition{
			"wiiid": {
				Type: database.SakeFieldTypeInt64,
			},
			"username": {
				Type: database.SakeFieldTypeBinaryData,
			},
			"friendkey": {
				Type: database.SakeFieldTypeInt64,
			},
		},
	},

	"mariokartwii/FriendInfo": {
		Rateable:         RateableNo,
		OwnerType:        OwnerTypeProfile,
		PublicPermCreate: PermissionAllowed,
		PublicPermRead:   PermissionAllowed,
		OwnerPermUpdate:  PermissionAllowed,
		OwnerPermDelete:  PermissionAllowed,
		LimitPerOwner:    1,
		Hardened:         false, // To allow modding extra fields
		Fields: map[string]SakeFieldDefinition{
			"info": {
				Type:                   database.SakeFieldTypeBinaryData,
				FilterFromDatabaseFunc: filterMarioKartWiiFriendInfo,
			},
		},
	},
	"mariokartwii/GhostData": {
		Rateable:                RateableNo,
		OwnerType:               OwnerTypeProfile,
		PublicPermCreate:        PermissionAllowed,
		PublicPermRead:          PermissionAllowed,
		OwnerPermUpdate:         PermissionAllowed,
		OwnerPermDelete:         PermissionAllowed,
		Hardened:                true,
		Reserved:                true,
		SearchForRecordsHandler: getMarioKartWiiGhostDataRecord,
		Fields: map[string]SakeFieldDefinition{
			"fileid": {
				Type: database.SakeFieldTypeInt,
			},
			"profile": {
				Type: database.SakeFieldTypeInt,
			},
			"course": {
				Type: database.SakeFieldTypeInt,
			},
			"region": {
				Type: database.SakeFieldTypeInt,
			},
			"time": {
				Type: database.SakeFieldTypeInt,
			},
		},
	},
	"mariokartwii/StoredGhostData": {
		Rateable:                RateableNo,
		OwnerType:               OwnerTypeProfile,
		PublicPermCreate:        PermissionAllowed,
		PublicPermRead:          PermissionAllowed,
		OwnerPermUpdate:         PermissionAllowed,
		OwnerPermDelete:         PermissionAllowed,
		Hardened:                true,
		Reserved:                true,
		SearchForRecordsHandler: getMarioKartWiiStoredGhostDataRecord,
		Fields: map[string]SakeFieldDefinition{
			"fileid": {
				Type: database.SakeFieldTypeInt,
			},
			"profile": {
				Type: database.SakeFieldTypeInt,
			},
			"course": {
				Type: database.SakeFieldTypeInt,
			},
			"region": {
				Type: database.SakeFieldTypeInt,
			},
			"time": {
				Type: database.SakeFieldTypeInt,
			},
		},
	},

	"guinnesswrds/RecordTable": {
		Rateable:         RateableUnknown,
		OwnerType:        OwnerTypeProfile,
		PublicPermCreate: PermissionAllowed,
		PublicPermRead:   PermissionAllowed,
		OwnerPermUpdate:  PermissionAllowed,
		OwnerPermDelete:  PermissionAllowed,
		Fields: map[string]SakeFieldDefinition{
			"Score": {
				Type: database.SakeFieldTypeInt,
			},
			"GameID": {
				Type: database.SakeFieldTypeByte,
			},
			"Region": {
				Type: database.SakeFieldTypeByte,
			},
			"Country": {
				Type: database.SakeFieldTypeByte,
			},
			"OwnerName": {
				Type: database.SakeFieldTypeUnicodeString,
			},
			"AvatarName": {
				Type: database.SakeFieldTypeUnicodeString,
			},
			"AvatarModel": {
				Type: database.SakeFieldTypeByte,
			},
			"AvatarParts": {
				Type: database.SakeFieldTypeBinaryData,
			},
			"DateTimeSet": {
				Type:    database.SakeFieldTypeDateAndTime,
				Default: "{CURRENT_TIMESTAMP}",
			},
		},
	},
}

func GetTable(gameName string, tableId string) *SakeTable {
	tableDef, exists := TableDefinitions[gameName+"/"+tableId]
	if !exists {
		return nil
	}
	return &tableDef
}

func (t *SakeTable) AllowsPublicCreate() bool {
	if t == nil {
		return true
	}
	if t.PublicPermCreate == PermissionDefault {
		return t.OwnerType == OwnerTypeProfile
	}
	return t.PublicPermCreate == PermissionAllowed
}

func (t *SakeTable) AllowsPublicRead() bool {
	if t == nil {
		return true
	}
	return t.PublicPermRead == PermissionAllowed || t.PublicPermRead == PermissionDefault
}

func (t *SakeTable) AllowsOwnerUpdate() bool {
	if t == nil {
		return true
	}
	return t.OwnerPermUpdate == PermissionAllowed || t.OwnerPermUpdate == PermissionDefault
}

func (t *SakeTable) AllowsOwnerDelete() bool {
	if t == nil {
		return true
	}
	return t.OwnerPermDelete == PermissionAllowed || t.OwnerPermDelete == PermissionDefault
}

func (t *SakeTable) GetDefaultFields() map[string]database.SakeField {
	if t == nil {
		return nil
	}
	defaultFields := make(map[string]database.SakeField)
	for fieldName, fieldDef := range t.Fields {
		if fieldDef.Default != "" {
			value := fieldDef.Default
			if value == "{CURRENT_TIMESTAMP}" && fieldDef.Type == database.SakeFieldTypeDateAndTime {
				value = time.Now().UTC().Format(DateAndTimeFormat)
			} else if value == "{EMPTY}" {
				value = ""
			}

			defaultFields[fieldName] = database.SakeField{
				Type:  fieldDef.Type,
				Value: value,
			}
		}
	}
	if t.Rateable == RateableYes {
		defaultFields["average_rating"] = database.SakeField{
			Type:  database.SakeFieldTypeFloat,
			Value: "0",
		}
		defaultFields["num_ratings"] = database.SakeField{
			Type:  database.SakeFieldTypeInt,
			Value: "0",
		}
	}

	return defaultFields
}

// MissingRequiredFields returns the names (sorted) of the fields a create request must carry but does not.
func (t *SakeTable) MissingRequiredFields(provided []string) []string {
	if t == nil || !t.RequireAllFields {
		return nil
	}
	have := map[string]bool{}
	for _, name := range provided {
		have[name] = true
	}
	var missing []string
	for name, def := range t.Fields {
		if def.Default == "" && !have[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

func (t *SakeTable) CheckValidField(fieldName string, field database.SakeField) Result {
	lengthLimit := MaxSakeFieldValueLength
	var verifyFunc func(value string) bool
	if t != nil && len(t.Fields) != 0 {
		fieldDef, exists := t.Fields[fieldName]
		if !exists {
			if t.Hardened {
				return ResultFieldNotFound
			}
		} else if fieldDef.Type != field.Type {
			return ResultFieldTypeInvalid
		}
		if fieldDef.LengthLimit > 0 {
			lengthLimit = fieldDef.LengthLimit
		}
		verifyFunc = fieldDef.IsValidFunc
	}
	if field.Type == database.SakeFieldTypeUnicodeString || field.Type == database.SakeFieldTypeAsciiString {
		// String limits count characters after decoding, with the byte cap as a backstop
		if len(field.Value) > MaxSakeFieldValueLength || utf8.RuneCountInString(field.Value) > lengthLimit {
			return ResultFieldTypeInvalid
		}
	} else if len(field.Value) > lengthLimit {
		return ResultFieldTypeInvalid
	}

	// These values may not be written to
	if fieldName == "average_rating" || fieldName == "my_rating" || fieldName == "num_ratings" || fieldName == "sum_ratings" || fieldName == "update_time" {
		return ResultFieldNotFound
	}

	switch field.Type {
	case database.SakeFieldTypeByte:
		if len(field.Value) == 0 || len(field.Value) > 3 {
			return ResultFieldTypeInvalid
		}
		if parsed, err := strconv.ParseUint(field.Value, 10, 8); err != nil || strconv.FormatUint(parsed, 10) != field.Value {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeShort:
		if len(field.Value) == 0 || len(field.Value) > 6 {
			return ResultFieldTypeInvalid
		}
		if parsed, err := strconv.ParseInt(field.Value, 10, 16); err != nil || strconv.FormatInt(parsed, 10) != field.Value {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeInt:
		if len(field.Value) == 0 || len(field.Value) > 11 {
			return ResultFieldTypeInvalid
		}
		if parsed, err := strconv.ParseInt(field.Value, 10, 32); err != nil || strconv.FormatInt(parsed, 10) != field.Value {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeInt64:
		if len(field.Value) == 0 || len(field.Value) > 20 {
			return ResultFieldTypeInvalid
		}
		if parsed, err := strconv.ParseInt(field.Value, 10, 64); err != nil || strconv.FormatInt(parsed, 10) != field.Value {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeFloat:
		if len(field.Value) == 0 || len(field.Value) > 24 {
			return ResultFieldTypeInvalid
		}
		_, err := strconv.ParseFloat(field.Value, 32)
		if err != nil {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeBoolean:
		if field.Value != "true" && field.Value != "false" && field.Value != "1" && field.Value != "0" {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeDateAndTime:
		if len(field.Value) == 0 || len(field.Value) > 24 {
			return ResultFieldTypeInvalid
		}
		_, err := time.Parse(DateAndTimeFormat, field.Value)
		if err != nil {
			return ResultFieldTypeInvalid
		}

	case database.SakeFieldTypeBinaryData:
		if len(field.Value) == 0 {
			return ResultSuccess
		}
		binaryData, err := base64.StdEncoding.Strict().DecodeString(field.Value)
		if err != nil {
			return ResultFieldTypeInvalid
		}
		if len(binaryData) > lengthLimit {
			return ResultFieldTypeInvalid
		}
	}

	if verifyFunc != nil && !verifyFunc(field.Value) {
		return ResultFieldTypeInvalid
	}
	return ResultSuccess
}

func (t *SakeTable) FilterFieldFromClient(fieldName string, value string) (string, Result) {
	if t == nil || t.Fields == nil {
		return value, ResultSuccess
	}
	fieldDef, exists := t.Fields[fieldName]
	if !exists {
		return value, ResultSuccess
	}
	if fieldDef.FilterFromClientFunc == nil {
		return value, ResultSuccess
	}
	return fieldDef.FilterFromClientFunc(value, true)
}

func (t *SakeTable) FilterFieldFromDatabase(fieldName string, value string, isOwner bool) (string, Result) {
	if t == nil || t.Fields == nil {
		return value, ResultSuccess
	}
	fieldDef, exists := t.Fields[fieldName]
	if !exists {
		return value, ResultSuccess
	}
	if fieldDef.FilterFromDatabaseFunc == nil {
		return value, ResultSuccess
	}
	if t.OwnerType != OwnerTypeProfile {
		isOwner = false
	}
	return fieldDef.FilterFromDatabaseFunc(value, isOwner)
}
