package ffcstats

import "testing"

func TestTeams(t *testing.T) {
	ab := []Pick{{Slot: 0, Creature: 102}, {Slot: 1, Creature: 53}}
	ba := []Pick{{Slot: 0, Creature: 53}, {Slot: 2, Creature: 102}, {Slot: 1, Creature: 53}} // same set, dup ignored
	abc := []Pick{{Creature: 7}, {Creature: 53}, {Creature: 102}}
	ms := []Match{
		{PidA: 1, PidB: 2, WinnerPid: 1, TeamA: ab, TeamB: abc},
		{PidA: 1, PidB: 3, WinnerPid: 3, TeamA: ba, TeamB: abc},
		{PidA: 2, PidB: 3, Draw: true, TeamA: ab, TeamB: nil}, // empty team B skipped
	}
	teams, pairs, total := Teams(ms, 1)
	if total != 5 || len(teams) != 2 {
		t.Fatal(total, teams)
	}
	top := teams[0]
	if len(top.Creatures) != 2 || top.Creatures[0] != 53 || top.Creatures[1] != 102 || top.Picks != 3 ||
		top.Wins != 1 || top.Losses != 1 || top.Draws != 1 {
		t.Fatalf("%+v", top)
	}
	if teams[1].Picks != 2 || teams[1].Wins != 1 || teams[1].Losses != 1 {
		t.Fatalf("%+v", teams[1])
	}
	// pairs: 53+102 on all 5 teams; 7+53 and 7+102 on the 2 abc teams
	if len(pairs) != 3 || pairs[0].Picks != 5 || pairs[0].Creatures[0] != 53 || pairs[1].Picks != 2 {
		t.Fatalf("%+v", pairs)
	}
	if teams, _, _ := Teams(ms, 3); len(teams) != 1 {
		t.Fatal("min filter", teams)
	}
}
