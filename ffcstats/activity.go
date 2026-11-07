package ffcstats

import "time"

// Activity (ffc-decomp ROADMAP #33 activity / peak-hours, #72 admin dashboard): match counts per UTC hour of day
// and per UTC day, from match rows (one row = one battle, any status the caller kept).
type Activity struct {
	ByHour  [24]int        `json:"by_hour"` // UTC hour of day, over the whole input
	ByDay   []DayCount     `json:"by_day"`  // the last `days` UTC days, oldest first (zero days included)
	Total   int            `json:"total"`
	Modded  int            `json:"modded"`
	Vanilla int            `json:"vanilla"`
	Status  map[string]int `json:"status"`
}

type DayCount struct {
	Day     string `json:"day"` // YYYY-MM-DD
	Matches int    `json:"matches"`
}

func ComputeActivity(ms []Match, now time.Time, days int) Activity {
	a := Activity{Status: map[string]int{}}
	if days < 1 {
		days = 1
	}
	today := now.UTC().Truncate(24 * time.Hour)
	first := today.AddDate(0, 0, -(days - 1))
	idx := map[string]int{}
	for i := 0; i < days; i++ {
		d := first.AddDate(0, 0, i).Format("2006-01-02")
		idx[d] = i
		a.ByDay = append(a.ByDay, DayCount{Day: d})
	}
	for _, m := range ms {
		t := m.CreatedAt.UTC()
		a.Total++
		a.ByHour[t.Hour()]++
		a.Status[m.Status]++
		if m.Modded {
			a.Modded++
		} else {
			a.Vanilla++
		}
		if i, ok := idx[t.Format("2006-01-02")]; ok {
			a.ByDay[i].Matches++
		}
	}
	return a
}

// PeakHour is the busiest UTC hour (-1 when there are no matches).
func (a Activity) PeakHour() int {
	best, h := 0, -1
	for i, n := range a.ByHour {
		if n > best {
			best, h = n, i
		}
	}
	return h
}
