package sake

import (
	"encoding/xml"
	"strings"
	"testing"
	"wwfc/database"
)

func ffcField(name, tag, value string) RecordField {
	return RecordField{Name: name, Value: RecordValue{Value: CommonValue{XMLName: xml.Name{Local: tag}, Value: value}}}
}

// TestFFCTblMainNaCaps (NET-39 b): CreateRecord/UpdateRecord input with an
// over-long name or an out-of-range group index is rejected for
// dental2ds/tbl_main_na. Lengths count characters after XML decoding.
func TestFFCTblMainNaCaps(t *testing.T) {
	table := GetTable("dental2ds", "tbl_main_na")
	if table == nil {
		t.Fatal("dental2ds/tbl_main_na not defined")
	}
	cases := []struct {
		field RecordField
		ok    bool
	}{
		{ffcField("player_name", "unicodeStringValue", "HUNTER"), true},
		{ffcField("player_name", "unicodeStringValue", "ABCDEFGH"), true},
		{ffcField("player_name", "unicodeStringValue", "ハンターハンター"), true}, // 8 chars, 24 bytes
		{ffcField("player_name", "unicodeStringValue", "ABCDEFGHI"), false},
		{ffcField("player_name", "unicodeStringValue", strings.Repeat("A", 200)), false},
		{ffcField("team_name", "unicodeStringValue", "Frigis Ghost"), true},
		{ffcField("team_name", "unicodeStringValue", strings.Repeat("T", 20)), true},
		{ffcField("team_name", "unicodeStringValue", strings.Repeat("T", 21)), false},
		{ffcField("rev1_name", "unicodeStringValue", strings.Repeat("N", 16)), true},
		{ffcField("rev3_name", "unicodeStringValue", strings.Repeat("N", 17)), false},
		{ffcField("rev5_name", "unicodeStringValue", strings.Repeat("N", 40)), false},
		{ffcField("group1", "intValue", "0"), true},
		{ffcField("group1", "intValue", "8"), true},
		{ffcField("group1", "intValue", "9"), false},
		{ffcField("group2", "intValue", "-1"), false},
		{ffcField("group2", "intValue", "100"), false},
	}
	for _, c := range cases {
		for _, create := range []bool{true, false} {
			req := StorageRequestCommon{TableID: "tbl_main_na"}
			req.Values.RecordFields = []RecordField{c.field}
			_, res := getInputFields("test", req, table, create)
			if got := res == ResultSuccess; got != c.ok {
				t.Errorf("%s=%q (create=%v): accepted=%v, want %v", c.field.Name, c.field.Value.Value.Value, create, got, c.ok)
			}
		}
	}
	// Direct check too (same function both handlers use).
	if table.CheckValidField("group1", database.SakeField{Type: database.SakeFieldTypeInt, Value: "9"}) == ResultSuccess {
		t.Error("CheckValidField accepted group1=9")
	}
}
