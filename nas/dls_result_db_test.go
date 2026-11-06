package nas

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"testing"
	"wwfc/common"
	"wwfc/database"
)

// Live Postgres check (skipped unless FFC_TEST_PG=1; local db wwfc/CHANGEME@127.0.0.1, schema.sql loaded): the two
// consoles' reports of one match end in ONE row, status confirmed; a contradicting pair is flagged mismatch.
func TestResultPostgresPair(t *testing.T) {
	if os.Getenv("FFC_TEST_PG") != "1" {
		t.Skip("FFC_TEST_PG != 1")
	}
	db := database.Start(common.Config{Username: "wwfc", Password: "CHANGEME", DatabaseAddress: "127.0.0.1", DatabaseName: "wwfc"})
	defer db.Close()
	if err := db.CreateMatchResultsTable(); err != nil {
		t.Fatal(err)
	}
	old := resultDB
	resultDB = &db
	defer func() { resultDB = old }()

	f := goldenFields(t)
	a := append(f["attr1"], f["attr2"]...) // reporter aid 0 (pid ...101) lost
	mirror := func(result byte) []byte {   // the same match seen from aid 1 (pid ...202)
		b := append([]byte{}, a...)
		b[1], b[3] = result, 1
		copy(b[4:8], a[8:12])
		copy(b[8:12], a[4:8])
		binary.LittleEndian.PutUint32(b[resultCrcOfs:], crc32.ChecksumIEEE(b[:resultCrcOfs]))
		return b
	}
	send := func(raw []byte, user string) string {
		return string(dlsResult("t", map[string][]byte{"attr1": raw[:96], "attr2": raw[96:], "userid": []byte(user)}))
	}
	if r := send(a, "4242"); len(r) < 3 || r[:2] != "ok" || r[len(r)-6:] != "single" {
		t.Fatalf("first report: %q", r)
	}
	if r := send(mirror(0), "777"); r[len(r)-9:] != "confirmed" {
		t.Fatalf("second report: %q", r)
	}
	send(a, "4242")
	if r := send(mirror(1), "777"); r[len(r)-8:] != "mismatch" { // both claim to have lost
		t.Fatalf("contradicting report: %q", r)
	}
}
