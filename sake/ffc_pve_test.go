package sake

import (
	"math/rand"
	"testing"

	"wwfc/common"
	"wwfc/database"
)

func TestPickPVERecord(t *testing.T) {
	pveRand = func(n int) int { return n - 1 }
	defer func() { pveRand = rand.Intn }()
	recs := []database.SakeRecord{{OwnerId: 5, RecordId: 1}, {OwnerId: 7, RecordId: 2}, {OwnerId: pveServerOwner, RecordId: 3}}
	if r, ok := pickPVERecord(recs, 5); !ok || r.RecordId != 3 {
		t.Fatal("server team first", r)
	}
	if r, ok := pickPVERecord(recs[:2], 5); !ok || r.RecordId != 2 {
		t.Fatal("other player's team, never own", r)
	}
	if _, ok := pickPVERecord(recs[:1], 5); ok {
		t.Fatal("own team only -> none")
	}
	g := common.GameInfo{Name: pveGameName}
	if !isPVERequest(g, []int32{PVERecordID}) || isPVERequest(g, []int32{PVERecordID, 1}) || isPVERequest(common.GameInfo{Name: "x"}, []int32{PVERecordID}) {
		t.Fatal("isPVERequest")
	}
	b := ratePVERecord("t", 1, StorageRequestCommon{Rating: 100})
	if b.RateRecordResponse == nil || b.RateRecordResponse.RateRecordResult != ResultAlreadyRated {
		t.Fatal("rate answer", b)
	}
}
