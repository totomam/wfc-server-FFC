package nas

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"strconv"
	"wwfc/common"
	"wwfc/database"
	"wwfc/logging"
	"wwfc/qr2"

	"github.com/logrusorgru/aurora/v3"
)

// PvP result reporting (ffc-decomp ROADMAP #20). The patched FFC client (mods/net_balance/hooks/net_result.c) posts
// Nd action "result" when an online battle ends; attr1 + attr2 carry the 192-byte report (each field DWC-base64,
// already decoded by parseAuthRequest). Layout + every field's game source: docs/sessions/s95-online/payload_spec.md.

const (
	resultLen       = 192
	resultHdrLen    = 20
	resultPlayerLen = 84
	resultCrcOfs    = 188
	resultFlagTime  = 1
	resultFlagMod   = 2
)

type ResultCreature struct {
	Slot     int      `json:"slot"`
	Creature uint8    `json:"creature"`
	Rank     uint8    `json:"rank_index"`
	BaseBP   uint16   `json:"base_bp"`
	PartBP   [4]uint8 `json:"part_bp"`
	Evo      uint8    `json:"evo"`
	Flags    uint8    `json:"flags"`
}

type ResultPlayer struct {
	Name        string           `json:"name"`
	ProfileWord uint32           `json:"profile_word"`
	Team        []ResultCreature `json:"team"`
}

type ResultReport struct {
	Version  uint8
	Result   uint8 // 0 reporter won, 1 reporter lost, 2 draw
	Flags    uint8
	MyAID    uint8
	MyPID    uint32
	OppPID   uint32
	MatchVer uint32
	Wins     uint16
	Losses   uint16
	Players  [2]ResultPlayer // aid order
}

// resultStore is the database part dlsResult needs (a fake in tests).
type resultStore interface {
	UserOwnsProfile(userID uint64, profileID uint32) (bool, error)
	ProfileExists(profileID uint32) (bool, error)
	StoreMatchReportDetail(r database.MatchReport) (database.StoreResult, error)
}

var resultDB resultStore

func parseResultReport(b []byte) (ResultReport, error) {
	var r ResultReport
	if len(b) != resultLen {
		return r, fmt.Errorf("length %d", len(b))
	}
	if crc32.ChecksumIEEE(b[:resultCrcOfs]) != binary.LittleEndian.Uint32(b[resultCrcOfs:]) {
		return r, errors.New("crc")
	}
	r.Version, r.Result, r.Flags, r.MyAID = b[0], b[1], b[2], b[3]
	r.MyPID = binary.LittleEndian.Uint32(b[4:])
	r.OppPID = binary.LittleEndian.Uint32(b[8:])
	r.MatchVer = binary.LittleEndian.Uint32(b[12:])
	r.Wins = binary.LittleEndian.Uint16(b[16:])
	r.Losses = binary.LittleEndian.Uint16(b[18:])
	switch {
	case r.Version != 1:
		return r, fmt.Errorf("version %d", r.Version)
	case r.Result > 2:
		return r, fmt.Errorf("result %d", r.Result)
	case r.MyAID > 1:
		return r, fmt.Errorf("aid %d", r.MyAID)
	case r.Flags&^(resultFlagTime|resultFlagMod) != 0:
		return r, fmt.Errorf("flags %#x", r.Flags)
	case r.Result == 2 && r.Flags&resultFlagTime == 0:
		return r, errors.New("draw without time limit") // payload_spec.md section 2: a draw only exists at the limit
	case r.MyPID == 0 || r.OppPID == 0 || r.MyPID == r.OppPID:
		return r, errors.New("profile ids")
	}
	for a := 0; a < 2; a++ {
		p := b[resultHdrLen+a*resultPlayerLen : resultHdrLen+(a+1)*resultPlayerLen]
		pl := ResultPlayer{Name: common.UTF16Decode(p[0:18], binary.LittleEndian), ProfileWord: binary.LittleEndian.Uint32(p[20:])}
		pl.Team = []ResultCreature{}
		for k := 0; k < 5; k++ {
			c := p[24+k*12 : 36+k*12]
			if c[0] == 0 {
				continue
			}
			pl.Team = append(pl.Team, ResultCreature{Slot: k, Creature: c[0], Rank: c[1], BaseBP: binary.LittleEndian.Uint16(c[2:]),
				PartBP: [4]uint8{c[4], c[5], c[6], c[7]}, Evo: c[8], Flags: c[9]})
		}
		if len(pl.Team) == 0 || int(p[18]) != len(pl.Team) {
			return r, fmt.Errorf("team %d", a)
		}
		r.Players[a] = pl
	}
	return r, nil
}

// toMatchReport normalises one report to the pair order A = lower profile id.
func (r ResultReport) toMatchReport(raw []byte) database.MatchReport {
	me, opp := r.Players[r.MyAID], r.Players[r.MyAID^1]
	m := database.MatchReport{TimeUp: r.Flags&resultFlagTime != 0, Modded: r.Flags&resultFlagMod != 0, MatchVer: r.MatchVer, Raw: raw}
	switch r.Result {
	case 0:
		m.WinnerPid = r.MyPID
	case 1:
		m.WinnerPid = r.OppPID
	default:
		m.Draw = true
	}
	teamMe, _ := json.Marshal(me.Team)
	teamOpp, _ := json.Marshal(opp.Team)
	if r.MyPID < r.OppPID {
		m.PidA, m.PidB, m.NameA, m.NameB, m.TeamA, m.TeamB, m.ReporterIsA = r.MyPID, r.OppPID, me.Name, opp.Name, teamMe, teamOpp, true
	} else {
		m.PidA, m.PidB, m.NameA, m.NameB, m.TeamA, m.TeamB = r.OppPID, r.MyPID, opp.Name, me.Name, teamOpp, teamMe
	}
	return m
}

// dlsResult answers action=result: "ok <row id> <status>" or "err <reason>" (the client only checks that a body came
// back; the reply is for logs and tests).
// matchQueueOf (s99 #35, server-only ranked/casual split): the QR2 group holding both players at report time.
// "Battle with Anyone" groups have dwc_mtype 0/1 (qr2 "anybody") -> "ranked"; friend battles connect to a friend's
// game server (dwc_mtype 2/3, "private") -> "casual"; no shared group (already left) -> "" (counted as ranked).
var matchQueueOf = func(a, b uint32) string {
	return queueFromGroups(qr2.GetGroups([]string{"dental2ds"}, []string{}, false), a, b)
}

// resultPairSeen (NET-10, human decision 2026-11-10 c): the two profiles were in one QR2 group within
// qr2.PairWindow (30 min default; a disconnect after grouping still counts). nas and qr2 share the backend process.
var resultPairSeen = qr2.SeenTogether

func queueFromGroups(groups []qr2.GroupInfo, a, b uint32) string {
	sa, sb := strconv.FormatUint(uint64(a), 10), strconv.FormatUint(uint64(b), 10)
	for _, g := range groups {
		var hasA, hasB bool
		for _, p := range g.Players {
			hasA = hasA || p.ProfileID == sa
			hasB = hasB || p.ProfileID == sb
		}
		if hasA && hasB {
			switch g.MatchType {
			case "anybody":
				return "ranked"
			case "private":
				return "casual"
			}
			return ""
		}
	}
	return ""
}

func dlsResult(moduleName string, fields map[string][]byte) []byte {
	raw := append(append([]byte{}, fields["attr1"]...), fields["attr2"]...)
	r, err := parseResultReport(raw)
	if err != nil {
		logging.Error(moduleName, "result: invalid report:", aurora.Cyan(err.Error()))
		return []byte("err report")
	}
	userID, ok := dlsUser(fields) // NET-10: userid + its SVCLOC token
	if !ok {
		logging.Error(moduleName, "result: userid", aurora.Cyan(string(fields["userid"])), "without a valid token")
		return []byte("err token")
	}
	if resultDB == nil {
		logging.Error(moduleName, "result: no database")
		return []byte("err db")
	}
	if ok, err := resultDB.UserOwnsProfile(userID, r.MyPID); err != nil || !ok {
		logging.Error(moduleName, "result: profile", aurora.Cyan(r.MyPID), "is not user", aurora.Cyan(userID))
		return []byte("err owner")
	}
	if ok, err := resultDB.ProfileExists(r.OppPID); err != nil || !ok {
		logging.Error(moduleName, "result: unknown opponent", aurora.Cyan(r.OppPID))
		return []byte("err opponent")
	}
	if !resultPairSeen(r.MyPID, r.OppPID) { // NET-10: only a pair QR2 grouped together recently
		logging.Error(moduleName, "result: profiles", aurora.Cyan(r.MyPID), "and", aurora.Cyan(r.OppPID), "were not in a match group together")
		return []byte("err pair")
	}
	m := r.toMatchReport(raw)
	m.Queue = matchQueueOf(m.PidA, m.PidB)
	res, err := resultDB.StoreMatchReportDetail(m)
	if errors.Is(err, database.ErrPairCap) { // NET-11: one pair cannot post rows forever
		logging.Warn(moduleName, "result: pair", aurora.Cyan(m.PidA), aurora.Cyan(m.PidB), "over", database.MatchPairCapPerHour, "rows/hour")
		return []byte("err cap")
	}
	if err != nil {
		logging.Error(moduleName, "result: store:", err)
		return []byte("err store")
	}
	id, status := res.ID, res.Status
	outcome := "draw"
	if !m.Draw {
		outcome = "winner " + strconv.FormatUint(uint64(m.WinnerPid), 10)
	}
	logging.Notice(moduleName, "Match result", aurora.Cyan(id), aurora.Cyan(status).String()+":", aurora.Cyan(m.PidA), aurora.Cyan(m.NameA),
		"vs", aurora.Cyan(m.PidB), aurora.Cyan(m.NameB), "->", aurora.BrightGreen(outcome), "reporter", aurora.Cyan(r.MyPID),
		"teams", aurora.Cyan(string(m.TeamA)), aurora.Cyan(string(m.TeamB)))
	if status == "mismatch" {
		logging.Warn(moduleName, "Match result", aurora.Cyan(id), aurora.BrightRed("MISMATCH"), "on", aurora.Cyan(res.Mismatch).String()+":",
			aurora.Cyan(m.PidA), "vs", aurora.Cyan(m.PidB), "second reporter", aurora.Cyan(r.MyPID), "(admin: /api/ffc/admin/flagged)")
	}
	if ev, data := matchEvent(res, m, r.MyPID); ev != "" {
		logging.Event(ev, data)
	}
	return []byte("ok " + strconv.FormatInt(id, 10) + " " + status)
}

// matchEvent picks the event for a stored report (ffc-decomp ROADMAP #66 match feed, #28 alerts): "match_result"
// once per match (when the second console confirms it), "match_mismatch" when the two reports disagree; a first
// (single) report sends nothing, so a webhook gets one message per battle. Event names go in config.xml
// <eventTypes> of a webhook (or "all").
func matchEvent(res database.StoreResult, m database.MatchReport, reporter uint32) (string, map[string]any) {
	var ev string
	switch res.Status {
	case "confirmed":
		ev = "match_result"
	case "mismatch":
		ev = "match_mismatch"
	default:
		return "", nil
	}
	queue := "vanilla"
	if m.Modded {
		queue = "modded"
	}
	result := "Draw"
	switch {
	case m.Draw:
	case m.WinnerPid == m.PidA:
		result = m.NameA + " won"
	default:
		result = m.NameB + " won"
	}
	data := map[string]any{
		"Match ID":  res.ID,
		"Players":   fmt.Sprintf("%s (%d) vs %s (%d)", m.NameA, m.PidA, m.NameB, m.PidB),
		"Result":    result,
		"Queue":     queue,
		"Time up":   m.TimeUp,
		"Teams":     string(m.TeamA) + " vs " + string(m.TeamB),
		"Winner ID": m.WinnerPid,
	}
	if ev == "match_mismatch" {
		data["Mismatch"] = res.Mismatch
		data["Second reporter"] = reporter
	}
	return ev, data
}
