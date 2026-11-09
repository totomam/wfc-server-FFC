package nas

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"wwfc/ffctable"
)

func ffcoBytes(n int) []byte {
	rows := make([]byte, 0, 8*n)
	for i := 0; i < n; i++ {
		rows = append(rows, byte(i&1), 3)
		rows = binary.LittleEndian.AppendUint16(rows, uint16(i))
		rows = binary.LittleEndian.AppendUint32(rows, uint32(100+i))
	}
	b := []byte("FFCO\x01\x00")
	b = binary.LittleEndian.AppendUint16(b, uint16(n))
	b = binary.LittleEndian.AppendUint32(b, 5)
	b = binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(rows))
	return append(b, rows...)
}

func TestDlsTable(t *testing.T) {
	dir, pdir := t.TempDir(), t.TempDir()
	oldD, oldP := ffctable.Dir, packageDir
	ffctable.Dir, packageDir = dir, pdir
	defer func() { ffctable.Dir, packageDir = oldD, oldP }()
	if err := os.WriteFile(filepath.Join(pdir, modesFile), []byte("draft\tDraft\nnofile\tNo file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := func(mode string) []byte {
		return dlsTable("test", map[string][]byte{"rhgamecd": []byte("VDEE"), "attr1": []byte(mode)})
	}
	if !bytes.Equal(q(""), tableNone) {
		t.Fatal("no file must be empty")
	}
	base, draft := ffcoBytes(4), ffcoBytes(7)
	if err := ffctable.Install("VDEE", "", base); err != nil {
		t.Fatal(err)
	}
	if err := ffctable.Install("VDEE", "draft", draft); err != nil {
		t.Fatal(err)
	}
	if err := ffctable.Install("VDEE", "unlisted", ffcoBytes(2)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(q(""), base) || !bytes.Equal(q("default"), base) {
		t.Fatal("base")
	}
	if !bytes.Equal(q("draft"), draft) {
		t.Fatal("mode file")
	}
	for _, m := range []string{"unlisted", "nofile", "../x", "nope"} {
		if !bytes.Equal(q(m), tableNone) {
			t.Fatal("must be empty:", m)
		}
	}
	// a corrupt file on disk is never served
	bad := append([]byte(nil), base...)
	bad[20] ^= 1
	if err := os.WriteFile(filepath.Join(dir, "VDEE.ffco"), bad, 0o644); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(q(""), tableNone) {
		t.Fatal("corrupt served")
	}
	// removal = no overlay
	if err := ffctable.Install("VDEE", "draft", nil); err != nil || !bytes.Equal(q("draft"), tableNone) {
		t.Fatal("removed", err)
	}
}
