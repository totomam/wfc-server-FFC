package sake

import (
	"testing"
	"wwfc/ffcbonus"
)

func TestFFCBadgeName(t *testing.T) {
	old := ffcbonus.BadgeSource
	defer func() { ffcbonus.BadgeSource = old }()
	ffcbonus.BadgeSource = nil
	if got := ffcBadgeName("tbl_main_na", "team_name", 5, 9, "Frigis Ghost"); got != "Frigis Ghost" {
		t.Fatal("off:", got)
	}
	ffcbonus.BadgeSource = func(o uint32) string {
		if o == 5 {
			return "#1"
		}
		return ""
	}
	cases := []struct {
		table, field string
		owner        int32
		req          uint32
		in, want     string
	}{
		{"tbl_main_na", "team_name", 5, 9, "Frigis Ghost", "#1 Frigis Gh"},
		{"tbl_main_na", "team_name", 5, 9, "Rex", "#1 Rex"},
		{"tbl_main_na", "team_name", 5, 5, "Rex", "Rex"}, // own record: never badged
		{"tbl_main_na", "player_name", 5, 9, "BOB", "BOB"},
		{"tbl_main_na", "team_name", 6, 9, "Rex", "Rex"},
		{"other", "team_name", 5, 9, "Rex", "Rex"},
	}
	for _, c := range cases {
		if got := ffcBadgeName(c.table, c.field, c.owner, c.req, c.in); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}
