package sake

import (
	"fmt"
	"testing"
	"wwfc/database"
)

func rec(id int32, fields map[string]string) database.SakeRecord {
	r := database.SakeRecord{RecordId: id, OwnerId: id, Fields: map[string]database.SakeField{}}
	for k, v := range fields {
		r.Fields[k] = database.SakeField{Type: database.SakeFieldTypeInt, Value: v}
	}
	return r
}

func ids(rs []database.SakeRecord) string {
	s := ""
	for _, r := range rs {
		s += fmt.Sprint(r.RecordId, " ")
	}
	return s
}

func TestSortRecords(t *testing.T) {
	mk := func() []database.SakeRecord {
		return []database.SakeRecord{
			rec(1, map[string]string{"lv": "9", "n": "b"}),
			rec(2, map[string]string{"lv": "10", "n": "a"}),
			rec(3, map[string]string{"n": "c"}),             // no lv
			rec(4, map[string]string{"lv": "10", "n": "d"}), // tie with 2
			rec(5, map[string]string{"lv": "2.5", "n": "e"}),
		}
	}
	cases := []struct{ expr, want string }{
		{"lv", "5 1 2 4 3 "},      // numeric (9 < 10, not string order), missing last
		{"lv DESC", "2 4 1 5 3 "}, // ties by record id, missing still last
		{"lv desc, n desc", "4 2 1 5 3 "},
		{"n", "2 1 3 4 5 "},
		{"", "1 2 3 4 5 "}, // empty expression: record id order
		{"recordid desc", "5 4 3 2 1 "},
	}
	for _, c := range cases {
		rs := mk()
		sortRecords(rs, c.expr)
		if got := ids(rs); got != c.want {
			t.Errorf("sort %q: got %s want %s", c.expr, got, c.want)
		}
	}
}

func TestSortVirtualFields(t *testing.T) {
	a, b := rec(1, nil), rec(2, nil)
	a.NumRatings, a.SumRatings = 2, 400 // avg 200
	b.NumRatings, b.SumRatings = 1, 300 // avg 300
	rs := []database.SakeRecord{a, b}
	sortRecords(rs, "average_rating desc")
	if ids(rs) != "2 1 " {
		t.Errorf("average_rating desc: %s", ids(rs))
	}
	sortRecords(rs, "num_ratings desc")
	if ids(rs) != "1 2 " {
		t.Errorf("num_ratings desc: %s", ids(rs))
	}
}

func TestCompareSortValuesTransitive(t *testing.T) {
	vals := []string{"-1", "0", "2.5", "9007199254740992", "9007199254740993", "9007199254740992.0", "9223372036854775807",
		"1e19", "inf", "nan", "1e400", "-inf", "abc", "", "0x10", "1e3", "1000"}
	for _, x := range vals {
		if compareSortValues(x, x) != 0 {
			t.Errorf("%q not equal to itself", x)
		}
		for _, y := range vals {
			if compareSortValues(x, y) != -compareSortValues(y, x) {
				t.Errorf("not antisymmetric: %q %q", x, y)
			}
			for _, z := range vals {
				if compareSortValues(x, y) <= 0 && compareSortValues(y, z) <= 0 && compareSortValues(x, z) > 0 {
					t.Errorf("not transitive: %q <= %q <= %q but %q > %q", x, y, z, x, z)
				}
			}
		}
	}
	if compareSortValues("9007199254740993", "9007199254740992.0") <= 0 {
		t.Error("int above 2^53 must compare greater than the float 2^53")
	}
	if compareSortValues("5", "inf") >= 0 || compareSortValues("abc", "inf") >= 0 {
		t.Error("numbers sort before strings (inf is a string)")
	}
}

func TestApplyOffsetMax(t *testing.T) {
	var rs []database.SakeRecord
	for i := int32(1); i <= 10; i++ {
		rs = append(rs, rec(i, nil))
	}
	cases := []struct {
		off, max int32
		want     string
	}{
		{0, 0, "1 2 3 4 5 6 7 8 9 10 "},
		{0, 3, "1 2 3 "},
		{3, 3, "4 5 6 "},
		{8, 5, "9 10 "},
		{10, 5, ""},
		{99, 5, ""},
		{-4, 2, "1 2 "},
	}
	for _, c := range cases {
		if got := ids(applyOffsetMax(append([]database.SakeRecord(nil), rs...), c.off, c.max)); got != c.want {
			t.Errorf("offset %d max %d: got %q want %q", c.off, c.max, got, c.want)
		}
	}
}

func TestRecordLimitFor(t *testing.T) {
	if got := recordLimitFor(&SakeTable{LimitPerOwner: 1}); got != MaxSakeRecordsPerProfile {
		t.Errorf("unenforced LimitPerOwner must not be reported: %d", got)
	}
	if got := recordLimitFor(&SakeTable{LimitPerOwner: 1, EnforceLimit: true}); got != 1 {
		t.Errorf("enforced limit: %d", got)
	}
	if got := recordLimitFor(nil); got != MaxSakeRecordsPerProfile {
		t.Errorf("nil table: %d", got)
	}
}

func TestMissingRequiredFields(t *testing.T) {
	table := GetTable("dental2ds", "tbl_main_na")
	if table == nil || !table.RequireAllFields || !table.EnforceLimit || table.LimitPerOwner != 1 {
		t.Fatalf("dental2ds table not as expected: %+v", table)
	}
	var all []string
	for name, def := range table.Fields {
		if def.Default == "" {
			all = append(all, name)
		}
	}
	if len(all) != 82 {
		t.Errorf("expected 82 required fields (those CreateRecord writes), got %d", len(all))
	}
	if m := table.MissingRequiredFields(all); len(m) != 0 {
		t.Errorf("complete record rejected: %v", m)
	}
	if m := table.MissingRequiredFields(all[1:]); len(m) != 1 || m[0] != all[0] {
		t.Errorf("one missing field not reported: %v", m)
	}
	if m := table.MissingRequiredFields([]string{"player_level"}); len(m) != 81 {
		t.Errorf("single-field record: %d missing", len(m))
	}
	if m := (&SakeTable{Fields: table.Fields}).MissingRequiredFields(nil); m != nil {
		t.Error("tables without RequireAllFields must not require anything")
	}
}
