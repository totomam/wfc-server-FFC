// Package ffctable stores the live balance overlay ("FFCO" v1, ffc-decomp docs/sessions/s126-online/unsigned_table.md,
// s129-online). The data is unsigned: the server checks the wire format only (magic, version, n, length, CRC, file < 2);
// the console's signed package bounds-checks every row and ignores the whole overlay on any error.
package ffctable

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrTable = errors.New("table")

// Dir holds <game>.ffco (mode "" / "default") and <game>.<mode>.ffco, plus history/.
var Dir = "./balance"

const (
	HeaderLen = 16
	RowLen    = 8
	MaxRows   = 2000
)

func bad(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrTable, fmt.Sprintf(f, a...)) }

// Validate checks the wire format and returns the row count, serial and crc.
func Validate(b []byte) (rows int, serial, crc uint32, err error) {
	if len(b) < HeaderLen || string(b[:4]) != "FFCO" {
		return 0, 0, 0, bad("magic")
	}
	if binary.LittleEndian.Uint16(b[4:]) != 1 {
		return 0, 0, 0, bad("version")
	}
	n := int(binary.LittleEndian.Uint16(b[6:]))
	if n < 1 || n > MaxRows {
		return 0, 0, 0, bad("row count (1..%d)", MaxRows)
	}
	if len(b) != HeaderLen+RowLen*n {
		return 0, 0, 0, bad("length")
	}
	serial = binary.LittleEndian.Uint32(b[8:])
	crc = binary.LittleEndian.Uint32(b[12:])
	if crc32.ChecksumIEEE(b[HeaderLen:]) != crc {
		return 0, 0, 0, bad("crc")
	}
	for i := 0; i < n; i++ {
		if b[HeaderLen+RowLen*i] > 1 {
			return 0, 0, 0, bad("row %d: file", i)
		}
	}
	return n, serial, crc, nil
}

// ValidMode: 1..32 of a-z A-Z 0-9 _ - (same as the nas event / mode name rule).
func ValidMode(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// File: "" / "default" = <dir>/<game>.ffco, else <dir>/<game>.<mode>.ffco.
func File(game, mode string) (string, error) {
	if game == "" || strings.ContainsAny(game, "/\\.") {
		return "", bad("game")
	}
	if mode == "" || mode == "default" {
		return filepath.Join(Dir, game+".ffco"), nil
	}
	if !ValidMode(mode) {
		return "", bad("mode name (1..32 of a-z A-Z 0-9 _ -)")
	}
	return filepath.Join(Dir, game+"."+mode+".ffco"), nil
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".up-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// Install validates data and writes it atomically (+ a history copy). Empty data removes the file (= no overlay;
// history gets a "<file>.<unix>.removed" note).
func Install(game, mode string, data []byte) error {
	path, err := File(game, mode)
	if err != nil {
		return err
	}
	hist := filepath.Join(Dir, "history", filepath.Base(path)+fmt.Sprintf(".%d", time.Now().Unix()))
	if len(data) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return writeAtomic(hist+".removed", nil)
	}
	if _, _, _, err := Validate(data); err != nil {
		return err
	}
	if err := writeAtomic(path, data); err != nil {
		return err
	}
	return writeAtomic(hist, data)
}

type Installed struct {
	File   string `json:"file"`
	Mode   string `json:"mode"`
	Rows   int    `json:"rows"`
	Serial uint32 `json:"serial"`
	CRC    string `json:"crc"`
	Bytes  int    `json:"bytes"`
}

// List the game's installed overlays (invalid files are skipped), sorted by file name.
func List(game string) ([]Installed, error) {
	ents, err := os.ReadDir(Dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Installed{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Installed{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, game) || !strings.HasSuffix(name, ".ffco") {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, game), ".ffco")
		mode := "default"
		if mid != "" {
			if mid[0] != '.' || !ValidMode(mid[1:]) {
				continue
			}
			mode = mid[1:]
		}
		b, err := os.ReadFile(filepath.Join(Dir, name))
		if err != nil {
			continue
		}
		n, serial, crc, err := Validate(b)
		if err != nil {
			continue
		}
		out = append(out, Installed{name, mode, n, serial, fmt.Sprintf("%08x", crc), len(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}
