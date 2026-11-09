package ffctable

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

// Build makes a valid FFCO v1 body with n rows.
func Build(n int, serial uint32) []byte {
	rows := make([]byte, 0, 8*n)
	for i := 0; i < n; i++ {
		rows = append(rows, byte(i&1), 3)
		rows = binary.LittleEndian.AppendUint16(rows, uint16(i))
		rows = binary.LittleEndian.AppendUint32(rows, uint32(100+i))
	}
	b := []byte("FFCO\x01\x00")
	b = binary.LittleEndian.AppendUint16(b, uint16(n))
	b = binary.LittleEndian.AppendUint32(b, serial)
	b = binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(rows))
	return append(b, rows...)
}

func TestValidate(t *testing.T) {
	if n, s, _, err := Validate(Build(3, 9)); err != nil || n != 3 || s != 9 {
		t.Fatal(n, s, err)
	}
	if _, _, _, err := Validate(Build(2000, 1)); err != nil {
		t.Fatal("2000:", err)
	}
	mut := func(f func(b []byte) []byte) []byte { return f(Build(3, 1)) }
	cases := map[string][]byte{
		"magic":   mut(func(b []byte) []byte { b[0] = 'X'; return b }),
		"version": mut(func(b []byte) []byte { b[4] = 2; return b }),
		"crc":     mut(func(b []byte) []byte { b[20] ^= 1; return b }),
		"short":   mut(func(b []byte) []byte { return b[:len(b)-1] }),
		"long":    mut(func(b []byte) []byte { return append(b, 0) }),
		"file2":   nil,
		"n0":      Build(0, 1),
		"n2001":   Build(2001, 1),
		"tiny":    []byte("FFCO"),
	}
	r := Build(1, 1)
	r[16] = 2
	binary.LittleEndian.PutUint32(r[12:], crc32.ChecksumIEEE(r[16:]))
	cases["file2"] = r
	for k, b := range cases {
		if _, _, _, err := Validate(b); err == nil {
			t.Fatal("accepted:", k)
		}
	}
}

func TestInstallListFile(t *testing.T) {
	Dir = t.TempDir()
	p, _ := File("VDEE", "")
	if p != filepath.Join(Dir, "VDEE.ffco") {
		t.Fatal(p)
	}
	if p, _ := File("VDEE", "blue"); p != filepath.Join(Dir, "VDEE.blue.ffco") {
		t.Fatal(p)
	}
	if _, err := File("VDEE", "../x"); err == nil {
		t.Fatal("bad mode")
	}
	if err := Install("VDEE", "", Build(2, 1)); err != nil {
		t.Fatal(err)
	}
	if err := Install("VDEE", "blue", Build(3, 2)); err != nil {
		t.Fatal(err)
	}
	if err := Install("VDEE", "blue", []byte("junk")); err == nil {
		t.Fatal("junk")
	}
	l, err := List("VDEE")
	if err != nil || len(l) != 2 || l[0].Mode != "blue" || l[0].Rows != 3 || l[1].Mode != "default" || l[1].Bytes != 32 {
		t.Fatal(l, err)
	}
	if h, _ := filepath.Glob(filepath.Join(Dir, "history", "*")); len(h) == 0 {
		t.Fatal("history")
	}
	if err := Install("VDEE", "blue", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Dir, "VDEE.blue.ffco")); err == nil {
		t.Fatal("not removed")
	}
	if l, _ := List("VDEE"); len(l) != 1 {
		t.Fatal(l)
	}
}
