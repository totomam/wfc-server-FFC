package sake

import "wwfc/ffcbonus"

// FFC Plaza ghost badges (ffc-decomp ROADMAP #78, s104-online): when another player's FFC Team Plaza record is served,
// its team_name gets a short badge prefix ("#1 Frigis Gho"), from ffcbonus.BadgeSource (set by the api package: leaderboard top 3
// + admin badges.json). Server-only: the game shows the text it gets. Never applied to the requester's OWN records, so
// the game cannot write a badge back into its stored team name.

const (
	ffcBadgeTable  = "tbl_main_na"
	ffcBadgeField  = "team_name"
	FFCBadgeMaxLen = 12 // shown length kept within what the s77 ghost test displayed ("Frigis Ghost")
)

// ffcBadgeName: the served team name for a record (unchanged without a badge / for the requester's own record).
func ffcBadgeName(tableID string, field string, ownerID int32, requester uint32, name string) string {
	if ffcbonus.BadgeSource == nil || tableID != ffcBadgeTable || field != ffcBadgeField || uint32(ownerID) == requester {
		return name
	}
	b := ffcbonus.BadgeSource(uint32(ownerID))
	if b == "" {
		return name
	}
	s := b + " " + name
	if r := []rune(s); len(r) > FFCBadgeMaxLen {
		s = string(r[:FFCBadgeMaxLen])
	}
	return s
}
