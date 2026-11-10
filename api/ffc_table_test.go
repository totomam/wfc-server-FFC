package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wwfc/ffctable"
)

func ffcoBody(n int) []byte {
	rows := make([]byte, 0, 8*n)
	for i := 0; i < n; i++ {
		rows = append(rows, 0, 1)
		rows = binary.LittleEndian.AppendUint16(rows, uint16(i))
		rows = binary.LittleEndian.AppendUint32(rows, uint32(i))
	}
	b := []byte("FFCO\x01\x00")
	b = binary.LittleEndian.AppendUint16(b, uint16(n))
	b = binary.LittleEndian.AppendUint32(b, 3)
	b = binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(rows))
	return append(b, rows...)
}

func TestFFCTableHandler(t *testing.T) {
	apiSecret = "s3cret"
	ffctable.Dir = filepath.Join(t.TempDir(), "balance")
	b64 := base64.StdEncoding.EncodeToString
	post := func(secret, mode string, tbl []byte) (int, string) {
		body, _ := json.Marshal(map[string]string{"secret": secret, "mode": mode, "table": b64(tbl)})
		rec := httptest.NewRecorder()
		HandleFFCTable(rec, httptest.NewRequest("POST", "/api/ffc/admin/table", bytes.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	good := ffcoBody(5)
	if c, _ := post("no", "", good); c != 401 {
		t.Fatal("wrong secret:", c)
	}
	if c, _ := post("s3cret", "", good[:len(good)-1]); c != 400 {
		t.Fatal("short:", c)
	}
	badcrc := append([]byte(nil), good...)
	badcrc[17] ^= 1
	if c, b := post("s3cret", "", badcrc); c != 400 || !strings.Contains(b, "crc") {
		t.Fatal("crc:", c, b)
	}
	if c, _ := post("s3cret", "", ffcoBody(0)); c != 400 {
		t.Fatal("n=0:", c)
	}
	if c, _ := post("s3cret", "bad/mode", good); c != 400 {
		t.Fatal("mode:", c)
	}
	if c, b := post("s3cret", "", good); c != 200 {
		t.Fatal("upload:", c, b)
	}
	if c, b := post("s3cret", "blue", ffcoBody(2)); c != 200 {
		t.Fatal("mode upload:", c, b)
	}
	if got, _ := os.ReadFile(filepath.Join(ffctable.Dir, "VDEE.ffco")); !bytes.Equal(got, good) {
		t.Fatal("file bytes")
	}
	get := func(q string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCTable(rec, adminGET("/api/ffc/admin/table", q))
		return rec.Code, rec.Body.String()
	}
	if c, _ := get("no"); c != 401 {
		t.Fatal("get wrong secret:", c)
	}
	c, b := get("s3cret")
	var out struct {
		Tables []ffctable.Installed `json:"tables"`
	}
	if c != 200 || json.Unmarshal([]byte(b), &out) != nil || len(out.Tables) != 2 || out.Tables[0].Mode != "blue" ||
		out.Tables[1].Mode != "default" || out.Tables[1].Rows != 5 || out.Tables[1].Serial != 3 {
		t.Fatal("list:", c, b)
	}
	if c, _ := post("s3cret", "", nil); c != 200 {
		t.Fatal("remove:", c)
	}
	if _, err := os.Stat(filepath.Join(ffctable.Dir, "VDEE.ffco")); err == nil {
		t.Fatal("not removed")
	}
}
