package ffcstats

import (
	"sort"
	"time"
)

// Balance impact (ffc-decomp ROADMAP #29): the modded rules version is the match's dwc_mver (0x40000000 | crc of
// table + package, mods/net_balance BalanceMatchVersion), so grouping matches by it shows each published table's
// effect: per version its first/last match and vivosaur pick/win rates, oldest version first. Vanilla (90) is a
// group too (the baseline).
type RulesVersion struct {
	MatchVer  uint32     `json:"match_ver"`
	Modded    bool       `json:"modded"`
	Matches   int        `json:"matches"`
	First     time.Time  `json:"first"`
	Last      time.Time  `json:"last"`
	Vivosaurs []Vivosaur `json:"vivosaurs"`
}

func BalanceImpact(ms []Match, top int) []RulesVersion {
	groups := map[uint32][]Match{}
	for _, m := range ms {
		groups[m.MatchVer] = append(groups[m.MatchVer], m)
	}
	out := []RulesVersion{}
	for v, g := range groups {
		rv := RulesVersion{MatchVer: v, Modded: v != 90, Matches: len(g), First: g[0].CreatedAt, Last: g[0].CreatedAt}
		for _, m := range g {
			if m.CreatedAt.Before(rv.First) {
				rv.First = m.CreatedAt
			}
			if m.CreatedAt.After(rv.Last) {
				rv.Last = m.CreatedAt
			}
		}
		rv.Vivosaurs, _ = Vivosaurs(g)
		if top > 0 && len(rv.Vivosaurs) > top {
			rv.Vivosaurs = rv.Vivosaurs[:top]
		}
		out = append(out, rv)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].First.Equal(out[j].First) {
			return out[i].First.Before(out[j].First)
		}
		return out[i].MatchVer < out[j].MatchVer
	})
	return out
}
