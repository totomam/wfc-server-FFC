package sake

import (
	"testing"
	"wwfc/database"
)

// FFC Team Plaza ai_1..ai_4: client writes limited to the 6 in-game AI policies (0..5).
func TestFFCAiPolicyRange(t *testing.T) {
	table := TableDefinitions["dental2ds/tbl_main_na"]
	for _, c := range []struct {
		v  string
		ok bool
	}{{"0", true}, {"5", true}, {"6", false}, {"16", false}, {"-1", false}, {"99", false}} {
		got := table.CheckValidField("ai_2", database.SakeField{Type: database.SakeFieldTypeInt, Value: c.v}) == ResultSuccess
		if got != c.ok {
			t.Errorf("ai_2=%s: valid=%v, want %v", c.v, got, c.ok)
		}
	}
}
