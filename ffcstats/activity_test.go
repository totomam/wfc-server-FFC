package ffcstats

import (
	"testing"
	"time"
)

func TestComputeActivity(t *testing.T) {
	now := time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)
	ms := []Match{
		{CreatedAt: now.Add(-1 * time.Hour), Status: "confirmed", Modded: true}, // 14h today
		{CreatedAt: now.Add(-25 * time.Hour), Status: "single"},                 // 14h yesterday
		{CreatedAt: now.Add(-3 * time.Hour), Status: "confirmed"},               // 12h today
		{CreatedAt: now.AddDate(0, 0, -30), Status: "confirmed"},                // outside the day window
	}
	a := ComputeActivity(ms, now, 3)
	if a.Total != 4 || a.Modded != 1 || a.Vanilla != 3 || a.Status["confirmed"] != 3 || a.Status["single"] != 1 {
		t.Fatalf("%+v", a)
	}
	if len(a.ByDay) != 3 || a.ByDay[0].Day != "2026-11-05" || a.ByDay[2].Day != "2026-11-07" ||
		a.ByDay[2].Matches != 2 || a.ByDay[1].Matches != 1 || a.ByDay[0].Matches != 0 {
		t.Fatalf("%+v", a.ByDay)
	}
	if a.ByHour[14] != 2 || a.ByHour[12] != 1 || a.PeakHour() != 14 {
		t.Fatalf("%v", a.ByHour)
	}
	if (Activity{}).PeakHour() != -1 {
		t.Fatal("empty peak")
	}
}
