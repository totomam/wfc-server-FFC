package api

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wwfc/ffcboot"
)

func TestFFCBootHandler(t *testing.T) {
	apiSecret = "s3cret"
	ffcboot.Dir, ffcboot.ModeDir = filepath.Join(t.TempDir(), "boot"), t.TempDir()
	code := []byte(strings.Repeat("c", 64))
	h := sha512.Sum512(code)
	pr := binary.LittleEndian.AppendUint16(nil, 1)
	pr = binary.LittleEndian.AppendUint16(pr, 0)
	pr = binary.LittleEndian.AppendUint32(pr, 44)
	pr = binary.LittleEndian.AppendUint32(pr, 0x021D7E00)
	pr = binary.LittleEndian.AppendUint32(pr, uint32(len(code)))
	pr = binary.LittleEndian.AppendUint32(pr, 0)
	pr = append(pr, h[:32]...)
	m := []byte("FFCM\x02\x00")
	m = binary.LittleEndian.AppendUint16(m, 1)
	m = binary.LittleEndian.AppendUint32(m, 7)
	m = binary.LittleEndian.AppendUint32(m, uint32(48+len(pr)+64))
	m = append(m, "VDEE\x01\x00\x00\x00"...)
	m = append(m, make([]byte, 24)...)
	m = append(append(m, pr...), make([]byte, 64)...)
	b64 := base64.StdEncoding.EncodeToString
	post := func(body string) (int, string) {
		rec := httptest.NewRecorder()
		HandleFFCBoot(rec, httptest.NewRequest("POST", "/api/ffc/admin/boot", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	if c, _ := post(`{"secret":"no","manifest":"` + b64(m) + `","parts":["` + b64(code) + `"]}`); c != 401 {
		t.Fatal("wrong secret:", c)
	}
	if c, b := post(`{"secret":"s3cret","mode":"x","manifest":"` + b64(m[:100]) + `"}`); c != 400 || !strings.Contains(b, "length") {
		t.Fatal("bad manifest:", c, b)
	}
	c, b := post(`{"secret":"s3cret","mode":"blue","display":"Blue","manifest":"` + b64(m) + `","parts":["` + b64(code) + `"]}`)
	if c != 200 || !strings.Contains(b, `"VDEE.blue.bin"`) || !strings.Contains(b, `"listed":true`) {
		t.Fatal("upload:", c, b)
	}
	if got, _ := os.ReadFile(filepath.Join(ffcboot.Dir, "VDEE.blue.bin")); string(got) != string(m) {
		t.Fatal("manifest file")
	}
	rec := httptest.NewRecorder()
	HandleFFCBoot(rec, httptest.NewRequest("GET", "/api/ffc/admin/boot?secret=s3cret", nil))
	var out struct {
		Manifests []ffcboot.Installed `json:"manifests"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Manifests) != 1 || out.Manifests[0].Counter != 7 {
		t.Fatal("list:", rec.Code, rec.Body.String())
	}
	if c, _ := post(`{"secret":"s3cret","mode":"blue","manifest":"` + b64(m) + `"}`); c != 400 {
		t.Fatal("replay accepted:", c)
	}
	if c, b := post(`{"secret":"s3cret","mode":"blue","force":true,"manifest":"` + b64(m) + `"}`); c != 200 {
		t.Fatal("force:", c, b)
	}
	if c, _ := post(`{"secret":"s3cret","manifest":"` + strings.Repeat("A", 2<<20) + `"}`); c != 413 {
		t.Fatal("oversize body:", c)
	}
	// A window-sized body (above the 64 KiB generic admin cap) must still reach the boot handler.
	if c, _ := post(`{"secret":"s3cret","manifest":"` + strings.Repeat("A", 300<<10) + `"}`); c == 413 {
		t.Fatal("window-sized body refused by the generic cap")
	}
}
