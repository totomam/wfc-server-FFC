package ffcboot

import (
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rec(t uint16, body []byte) []byte {
	b := binary.LittleEndian.AppendUint16(nil, t)
	b = binary.LittleEndian.AppendUint16(b, 0)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(body)))
	b = append(b, body...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func partRec(addr uint32, img []byte) []byte {
	h := sha512.Sum512(img)
	b := binary.LittleEndian.AppendUint32(nil, addr)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(img)))
	b = binary.LittleEndian.AppendUint32(b, 0)
	return rec(1, append(b, h[:32]...))
}

// mkManifest: unsigned-but-shaped manifest (64 zero bytes in place of the signature: the server never checks it).
func mkManifest(counter uint32, recs ...[]byte) []byte {
	var body []byte
	for _, r := range recs {
		body = append(body, r...)
	}
	total := 48 + len(body) + 64
	h := []byte("FFCM")
	h = append(h, 2, 0)
	h = binary.LittleEndian.AppendUint16(h, uint16(len(recs)))
	h = binary.LittleEndian.AppendUint32(h, counter)
	h = binary.LittleEndian.AppendUint32(h, uint32(total))
	h = append(h, "VDEE"...)
	h = binary.LittleEndian.AppendUint32(h, 1)
	h = binary.LittleEndian.AppendUint32(h, 0)
	h = binary.LittleEndian.AppendUint32(h, 0)
	h = append(h, make([]byte, 16)...)
	return append(append(h, body...), make([]byte, 64)...)
}

func setup(t *testing.T) {
	Dir, ModeDir = filepath.Join(t.TempDir(), "boot"), t.TempDir()
}

func TestInstall(t *testing.T) {
	setup(t)
	code, table := []byte(strings.Repeat("c", 1000)), []byte(strings.Repeat("t", 333))
	m := mkManifest(1, partRec(0x021D7E00, code), partRec(0x021F6C00, table), rec(3, []byte("hack")))
	res, err := Install("VDEE", "", "", m, [][]byte{table, code}, false)
	if err != nil || res.PartsWritten != 2 || res.File != "VDEE.bin" || res.Label != "hack" || !res.Listed {
		t.Fatal(res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(Dir, "VDEE.bin")); string(b) != string(m) {
		t.Fatal("manifest not written")
	}
	for _, p := range []([]byte){code, table} {
		h := sha512.Sum512(p)
		n := filepath.Join(Dir, "parts", strings.ToLower(hexs(h[:8]))+".bin")
		if b, err := os.ReadFile(n); err != nil || string(b) != string(p) {
			t.Fatal("part", n, err)
		}
	}
	// replay floor
	if _, err := Install("VDEE", "default", "", m, nil, false); !errors.Is(err, ErrBoot) || !strings.Contains(err.Error(), "counter") {
		t.Fatal("same counter accepted:", err)
	}
	// new mode sharing the code part (left out: already on the server) + its own table; listed with a display name
	table2 := []byte(strings.Repeat("u", 444))
	m2 := mkManifest(1, partRec(0x021D7E00, code), partRec(0x021F6C00, table2))
	res, err = Install("VDEE", "test", "Test rules", m2, [][]byte{table2}, false)
	if err != nil || res.PartsWritten != 1 || res.File != "VDEE.test.bin" || !res.Listed {
		t.Fatal(res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(ModeDir, "modes.tsv")); string(b) != "test\tTest rules\n" {
		t.Fatalf("modes.tsv %q", b)
	}
	// rename keeps the row, a mode without display stays unlisted
	if _, err = Install("VDEE", "test", "Test 2", mkManifest(2, partRec(0x021D7E00, code)), nil, false); err != nil {
		t.Fatal(err)
	}
	res, err = Install("VDEE", "ev1", "", mkManifest(1, partRec(0x021D7E00, code)), nil, false)
	if err != nil || res.Listed {
		t.Fatal(res, err)
	}
	l, err := List("VDEE")
	if err != nil || len(l) != 3 {
		t.Fatal(l, err)
	}
	for _, it := range l {
		if len(it.Missing) != 0 {
			t.Fatal("missing", it)
		}
		if it.Mode == "test" && (it.Counter != 2 || !it.Listed) {
			t.Fatal(it)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(ModeDir, "modes.tsv")); string(b) != "test\tTest 2\n" {
		t.Fatalf("modes.tsv %q", b)
	}
}

func hexs(b []byte) string {
	const d = "0123456789abcdef"
	s := ""
	for _, c := range b {
		s += string(d[c>>4]) + string(d[c&15])
	}
	return s
}

func TestRefuse(t *testing.T) {
	setup(t)
	code := []byte(strings.Repeat("c", 100))
	good := mkManifest(1, partRec(0x021D7E00, code))
	cases := map[string]struct {
		m     []byte
		parts [][]byte
		mode  string
	}{
		"short":      {good[:100], [][]byte{code}, ""},
		"magic":      {append([]byte("XXXX"), good[4:]...), [][]byte{code}, ""},
		"hash":       {good, [][]byte{append(append([]byte{}, code[:99]...), 'x')}, ""},
		"missing":    {good, nil, ""},
		"extra":      {good, [][]byte{code, []byte("zz")}, ""},
		"window":     {mkManifest(1, partRec(0x02300000, code)), [][]byte{code}, ""},
		"overlap":    {mkManifest(1, partRec(0x021D7E00, code), partRec(0x021D7E10, []byte("abcd"))), [][]byte{code, []byte("abcd")}, ""},
		"no parts":   {mkManifest(1, rec(3, []byte("x"))), nil, ""},
		"mode name":  {good, [][]byte{code}, "../x"},
		"rec bounds": {func() []byte { b := append([]byte{}, good...); binary.LittleEndian.PutUint32(b[52:], 9999); return b }(), [][]byte{code}, ""},
	}
	for name, c := range cases {
		if _, err := Install("VDEE", c.mode, "", c.m, c.parts, false); !errors.Is(err, ErrBoot) {
			t.Error(name, "accepted:", err)
		}
	}
	if ents, _ := os.ReadDir(Dir); len(ents) != 0 {
		t.Fatal("refused uploads wrote files:", len(ents))
	}
	// 5th mode refused (manifest written, row not)
	os.MkdirAll(ModeDir, 0o755)
	os.WriteFile(filepath.Join(ModeDir, "modes.tsv"), []byte("default\tHack\na\tA\nb\tB\nc\tC\n"), 0o644)
	if res, err := Install("VDEE", "d", "D", good, [][]byte{code}, false); res == nil || !errors.Is(err, ErrBoot) || res.Listed {
		t.Fatal(res, err)
	}
}
