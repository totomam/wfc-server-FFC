package nas

import (
	"encoding/binary"
	"encoding/hex"
	"hash/crc32"
	"strconv"
	"strings"
	"testing"
	"wwfc/common"
	"wwfc/database"
	"wwfc/qr2"
)

// Golden report from the client builder (ffc-decomp mods/net_balance/test/net_result_test.c, last NrBuild call):
// aid 0 (pid 1000000101 "ALICE", 3 vivos 102..104) vs aid 1 (pid 1000000202 "BOB", 5 vivos 7..11), result 1
// (reporter lost), time-up + modded flags, mver 0x40001234.
const (
	goldenAttr1 = "AQEDAGXKmjvKypo7NBIAQAQAAgBBAEwASQBDAEUAAAAAAAAAAAADC3hWNBJmEmQAChQeKAIDAABnE8gACxUfKQIDAABoFCwBDBYgKgIDAAAAAAAAAAAAAAAAAAAAAAAA"
	goldenAttr2 = "AAAAAAAAAABCAE8AQgAAAAAAAAAAAAAAAAAFC3lWNBIHB2UAChQeKAIDAAAICMkACxUfKQIDAAAJCS0BDBYgKgIDAAAKCpEBDRchKwIDAAALC-UBDhgiLAIDAAA-Kivv"
	goldenHex   = "0101030065ca9a3bcaca9a3b341200400400020041004c004900430045000000000000000000030b78563412661264000a141e28020300006713c8000b151f290203000068142c010c16202a0203000000000000000000000000000000000000000000000000000042004f004200000000000000000000000000050b79563412070765000a141e28020300000808c9000b151f290203000009092d010c16202a020300000a0a91010d17212b020300000b0bf5010e18222c020300003f2a2bef"
)

type fakeResultDB struct {
	owners   map[uint32]uint64
	stored   []database.MatchReport
	lastErr  error
	status   string // returned status ("" = single)
	mismatch string
}

func (f *fakeResultDB) UserOwnsProfile(u uint64, p uint32) (bool, error) {
	return f.owners[p] == u, nil
}
func (f *fakeResultDB) ProfileExists(p uint32) (bool, error) { _, ok := f.owners[p]; return ok, nil }
func (f *fakeResultDB) StoreMatchReportDetail(r database.MatchReport) (database.StoreResult, error) {
	f.stored = append(f.stored, r)
	st := f.status
	if st == "" {
		st = "single"
	}
	return database.StoreResult{ID: int64(len(f.stored)), Status: st, Mismatch: f.mismatch}, f.lastErr
}

func goldenFields(t *testing.T) map[string][]byte {
	a1, err1 := common.Base64DwcEncoding.DecodeString(goldenAttr1)
	a2, err2 := common.Base64DwcEncoding.DecodeString(goldenAttr2)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return map[string][]byte{"attr1": a1, "attr2": a2, "userid": []byte("4242"), "token": []byte(dlsIssueToken(4242)), "rhgamecd": []byte("VDEE")}
}

func TestResultGoldenDecode(t *testing.T) {
	f := goldenFields(t)
	raw := append(f["attr1"], f["attr2"]...)
	if hex.EncodeToString(raw) != goldenHex {
		t.Fatal("DWC-base64 attr decode != client bytes")
	}
	r, err := parseResultReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if r.Result != 1 || r.MyAID != 0 || r.MyPID != 1000000101 || r.OppPID != 1000000202 || r.MatchVer != 0x40001234 || r.Flags != 3 {
		t.Fatalf("header %+v", r)
	}
	if r.Players[0].Name != "ALICE" || r.Players[1].Name != "BOB" || len(r.Players[0].Team) != 3 || len(r.Players[1].Team) != 5 {
		t.Fatalf("players %+v", r.Players)
	}
	c := r.Players[0].Team[1]
	if c.Creature != 103 || c.BaseBP != 200 || c.PartBP != [4]uint8{11, 21, 31, 41} || c.Evo != 2 || c.Slot != 1 {
		t.Fatalf("creature %+v", c)
	}
	m := r.toMatchReport(raw)
	if m.PidA != 1000000101 || m.WinnerPid != 1000000202 || m.Draw || !m.ReporterIsA || !m.TimeUp || !m.Modded || m.NameB != "BOB" {
		t.Fatalf("match %+v", m)
	}
}

func TestDlsResultValidation(t *testing.T) {
	fake := &fakeResultDB{owners: map[uint32]uint64{1000000101: 4242, 1000000202: 777}}
	old := resultDB
	resultDB = fake
	defer func() { resultDB = old }()

	if got := string(dlsResult("t", goldenFields(t))); got != "ok 1 single" || len(fake.stored) != 1 {
		t.Fatalf("valid report: %q", got)
	}
	f := goldenFields(t)
	f["userid"] = []byte("777") // someone else's user id claiming pid 1000000101
	f["token"] = []byte(dlsIssueToken(777))
	if got := string(dlsResult("t", f)); got != "err owner" {
		t.Fatalf("owner check: %q", got)
	}
	f = goldenFields(t)
	f["attr2"][10] ^= 1 // corrupt -> crc
	if got := string(dlsResult("t", f)); got != "err report" {
		t.Fatalf("crc check: %q", got)
	}
	delete(fake.owners, 1000000202)
	if got := string(dlsResult("t", goldenFields(t))); got != "err opponent" {
		t.Fatalf("opponent check: %q", got)
	}
}

func TestResultParseRules(t *testing.T) {
	f := goldenFields(t)
	base := append(f["attr1"], f["attr2"]...)
	mut := func(fn func(b []byte)) error {
		b := append([]byte{}, base...)
		fn(b)
		binary.LittleEndian.PutUint32(b[resultCrcOfs:], crc32.ChecksumIEEE(b[:resultCrcOfs]))
		_, err := parseResultReport(b)
		return err
	}
	cases := map[string]func(b []byte){
		"result 3":       func(b []byte) { b[1] = 3 },
		"draw no timeup": func(b []byte) { b[1] = 2; b[2] = 0 },
		"same pids":      func(b []byte) { copy(b[8:12], b[4:8]) },
		"zero pid":       func(b []byte) { copy(b[4:8], []byte{0, 0, 0, 0}) },
		"empty team": func(b []byte) {
			for k := 0; k < 5; k++ {
				b[resultHdrLen+24+k*12] = 0
			}
		},
		"count mismatch": func(b []byte) { b[resultHdrLen+18] = 4 },
		"aid 2":          func(b []byte) { b[3] = 2 },
		"version 2":      func(b []byte) { b[0] = 2 },
	}
	for name, fn := range cases {
		if err := mut(fn); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := mut(func(b []byte) { b[1] = 2 }); err != nil {
		t.Errorf("draw at time limit rejected: %v", err)
	}
	if _, err := parseResultReport(base[:100]); err == nil || !strings.Contains(err.Error(), "length") {
		t.Error("short report accepted")
	}
}

func TestCompareMatch(t *testing.T) {
	f := goldenFields(t)
	raw := append(f["attr1"], f["attr2"]...)
	r, _ := parseResultReport(raw)
	m := r.toMatchReport(raw)
	w := int64(m.WinnerPid)
	if s := database.CompareMatch(m, &w, false, true, m.MatchVer, m.TeamA, m.TeamB); s != "" {
		t.Fatalf("same report differs: %s", s)
	}
	l := int64(m.PidA)
	if s := database.CompareMatch(m, &l, false, true, m.MatchVer, m.TeamA, m.TeamB); s != "winner" {
		t.Fatalf("winner mismatch not flagged: %q", s)
	}
	if s := database.CompareMatch(m, &w, false, true, m.MatchVer, m.TeamB, m.TeamB); s != "team_a" {
		t.Fatalf("team mismatch not flagged: %q", s)
	}
	if s := database.CompareMatch(m, &w, false, !m.TimeUp, m.MatchVer, m.TeamA, m.TeamB); s != "" {
		t.Fatalf("time_up difference flagged (stale per-console byte, s119): %q", s)
	}
}

func TestQueueFromGroups(t *testing.T) {
	pl := func(ids ...string) map[string]qr2.PlayerInfo {
		m := map[string]qr2.PlayerInfo{}
		for i, id := range ids {
			m[strconv.Itoa(i)] = qr2.PlayerInfo{ProfileID: id}
		}
		return m
	}
	gs := []qr2.GroupInfo{{MatchType: "anybody", Players: pl("1", "2")}, {MatchType: "private", Players: pl("3", "4")},
		{MatchType: "unknown", Players: pl("5", "6")}, {MatchType: "anybody", Players: pl("7")}, {MatchType: "private", Players: pl("8")}}
	for _, c := range []struct {
		a, b uint32
		want string
	}{{1, 2, "ranked"}, {3, 4, "casual"}, {5, 6, ""}, {7, 8, ""}, {1, 4, ""}} {
		if got := queueFromGroups(gs, c.a, c.b); got != c.want {
			t.Errorf("%d,%d: %q want %q", c.a, c.b, got, c.want)
		}
	}
}
