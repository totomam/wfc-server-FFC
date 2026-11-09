package ffcboot

import (
	"bufio"
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Stage-0 package upload (ffc-decomp docs/online/STAGE0.md phase 4, s125-online). The admin API hands over a SIGNED
// manifest + its parts for a mode; this package checks the manifest's format and bounds (a Go twin of
// tools/online_boot.py parse / stage-0 S0Check) and every part's length and SHA-512 against it, then writes
// boot/parts/<hex16>.bin (content-addressed) first and boot/<game>[.<mode>].bin last, each via temp file + rename, so
// a console fetching at the same time sees either the old manifest (whose parts are still there) or the complete new
// one. The signature is NOT checked here: the server never holds a key, the console verifies it.

var (
	Dir     = "./boot"    // nas.bootDir
	ModeDir = "./package" // modes.tsv lives here (nas.packageDir)
)

const (
	magic       = "FFCM"
	format      = 2
	hdrLen      = 48
	sigLen      = 64
	ManifestMax = 16 * 1024 // stage-0 staging buffer
	maxRecords  = 32
	maxParts    = 8
	recPart     = 1
	recLabel    = 3
	modesFile   = "modes.tsv"
	maxModes    = 4
)

// Package windows (mods/net_stage0, tools/online_boot.py WIN): every part (len + bss) lies inside one.
var windows = [][2]uint32{{0x021D7E00, 0x02202C00}, {0x01FFFD40, 0x02000000}}

var mu sync.Mutex

var ErrBoot = errors.New("boot upload")

func bad(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrBoot, fmt.Sprintf(f, a...)) }

type Part struct {
	Addr, Len, Bss uint32
	Hash           [32]byte // SHA-512 truncated
}

// Name is the server file name of the part: hex of the first 8 SHA-512 bytes.
func (p Part) Name() string { return hex.EncodeToString(p.Hash[:8]) }

type Manifest struct {
	Counter uint32
	KeySlot uint8
	Label   string
	Parts   []Part
}

// Parse checks the header and walks every record (bounds only; unknown records are skipped like stage-0 does).
func Parse(m []byte) (*Manifest, error) {
	if len(m) < hdrLen+sigLen || len(m)%4 != 0 || len(m) > ManifestMax {
		return nil, bad("manifest length %d (48+64..16384, multiple of 4)", len(m))
	}
	if string(m[:4]) != magic || m[4] != format || m[5] > 1 || string(m[16:20]) != "VDEE" ||
		!bytes.Equal(m[28:hdrLen], make([]byte, hdrLen-28)) {
		return nil, bad("manifest header (FFCM format 2, key slot 0/1, VDEE, reserved 0)")
	}
	le := binary.LittleEndian
	nrec, counter, total := le.Uint16(m[6:]), le.Uint32(m[8:]), le.Uint32(m[12:])
	if int(total) != len(m) || nrec == 0 || nrec > maxRecords {
		return nil, bad("manifest total_len/nrec")
	}
	out := &Manifest{Counter: counter, KeySlot: m[5]}
	off, end := hdrLen, len(m)-sigLen
	for i := 0; i < int(nrec); i++ {
		if off+8 > end {
			return nil, bad("record %d bounds", i)
		}
		t, ln := le.Uint16(m[off:]), int(le.Uint32(m[off+4:]))
		if ln < 0 || ln > end-off-8 {
			return nil, bad("record %d bounds", i)
		}
		body := m[off+8 : off+8+ln]
		switch t {
		case recPart:
			if ln != 44 {
				return nil, bad("PART record length %d", ln)
			}
			p := Part{Addr: le.Uint32(body), Len: le.Uint32(body[4:]), Bss: le.Uint32(body[8:])}
			copy(p.Hash[:], body[12:44])
			out.Parts = append(out.Parts, p)
		case recLabel:
			out.Label = string(body)
		}
		off += 8 + (ln+3)&^3
	}
	if off != end {
		return nil, bad("trailing bytes before the signature")
	}
	if len(out.Parts) == 0 || len(out.Parts) > maxParts {
		return nil, bad("%d parts (1..%d)", len(out.Parts), maxParts)
	}
	type span struct{ lo, hi uint64 }
	var spans []span
	for i, p := range out.Parts {
		lo, hi := uint64(p.Addr), uint64(p.Addr)+uint64(p.Len)+uint64(p.Bss)
		in := false
		for _, w := range windows {
			in = in || (lo >= uint64(w[0]) && hi <= uint64(w[1]))
		}
		if !in || p.Len == 0 {
			return nil, bad("part %d 0x%08x+%d+%d outside the package windows", i, p.Addr, p.Len, p.Bss)
		}
		for _, s := range spans {
			if lo < s.hi && s.lo < hi {
				return nil, bad("part %d overlaps another part", i)
			}
		}
		spans = append(spans, span{lo, hi})
	}
	return out, nil
}

func validName(s string) bool {
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

func validDisplay(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// ManifestPath: "" / "default" = the base file <game>.bin, else <game>.<mode>.bin (nas modeFile).
func ManifestPath(game, mode string) (string, error) {
	if game != "VDEE" {
		return "", bad("game")
	}
	if mode == "" || mode == "default" {
		return filepath.Join(Dir, game+".bin"), nil
	}
	if !validName(mode) {
		return "", bad("mode name (1..32 of a-z A-Z 0-9 _ -)")
	}
	return filepath.Join(Dir, game+"."+mode+".bin"), nil
}

func writeAtomic(dir, name string, data []byte) error {
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
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

func hashOK(p Part, img []byte) bool {
	h := sha512.Sum512(img)
	return uint32(len(img)) == p.Len && bytes.Equal(h[:32], p.Hash[:])
}

type Result struct {
	File         string   `json:"file"`
	Counter      uint32   `json:"counter"`
	Label        string   `json:"label"`
	Parts        []string `json:"parts"`
	PartsWritten int      `json:"parts_written"`
	Bytes        int      `json:"bytes"`  // manifest + parts
	Listed       bool     `json:"listed"` // mode is in modes.tsv (served to Game Mode); always true for default
}

// Install checks manifest + parts and writes them (parts first, manifest last). parts may be given in any order;
// a part the manifest names may be omitted when boot/parts already holds it with the right hash. Extra parts are
// refused. force=false refuses a counter <= the installed manifest's (consoles that hold it would say REPLAY).
// display != "" adds/updates the mode's modes.tsv row (Game Mode list).
func Install(game, mode, display string, manifest []byte, parts [][]byte, force bool) (*Result, error) {
	path, err := ManifestPath(game, mode)
	if err != nil {
		return nil, err
	}
	isDefault := mode == "" || mode == "default"
	if display != "" && !validDisplay(display) {
		return nil, bad("display name (1..20 printable ASCII)")
	}
	man, err := Parse(manifest)
	if err != nil {
		return nil, err
	}
	byName := map[string][]byte{}
	for i, img := range parts {
		h := sha512.Sum512(img)
		n := hex.EncodeToString(h[:8])
		if _, dup := byName[n]; dup {
			return nil, bad("uploaded part %d is a duplicate", i)
		}
		byName[n] = img
	}
	mu.Lock()
	defer mu.Unlock()
	if old, err := os.ReadFile(path); err == nil && !force {
		if om, err := Parse(old); err == nil && man.Counter <= om.Counter {
			return nil, bad("counter %d <= installed %d (consoles holding it refuse: REPLAY); raise it or force",
				man.Counter, om.Counter)
		}
	}
	res := &Result{File: filepath.Base(path), Counter: man.Counter, Label: man.Label, Bytes: len(manifest)}
	write := map[string][]byte{}
	for i, p := range man.Parts {
		n := p.Name()
		res.Parts = append(res.Parts, n)
		if img, ok := byName[n]; ok {
			if !hashOK(p, img) {
				return nil, bad("part %d (%s): length/hash differs from the manifest", i, n)
			}
			delete(byName, n)
			if have, err := os.ReadFile(filepath.Join(Dir, "parts", n+".bin")); err != nil || !bytes.Equal(have, img) {
				write[n] = img
			}
			res.Bytes += len(img)
			continue
		}
		if _, done := write[n]; done {
			continue
		}
		have, err := os.ReadFile(filepath.Join(Dir, "parts", n+".bin"))
		if err != nil || !hashOK(p, have) {
			return nil, bad("part %d (%s) missing: not uploaded and not on the server", i, n)
		}
	}
	if len(byName) > 0 {
		return nil, bad("%d uploaded parts are not named by the manifest", len(byName))
	}
	names := make([]string, 0, len(write))
	for n := range write {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := writeAtomic(filepath.Join(Dir, "parts"), n+".bin", write[n]); err != nil {
			return nil, err
		}
	}
	res.PartsWritten = len(names)
	if err := writeAtomic(Dir, filepath.Base(path), manifest); err != nil {
		return nil, err
	}
	res.Listed = isDefault
	if !isDefault {
		if display != "" {
			if err := upsertMode(mode, display); err != nil {
				return res, err // manifest is installed; only the list row failed
			}
		}
		res.Listed = modeListed(mode)
	}
	return res, nil
}

func readModeLines() ([]string, error) {
	f, err := os.Open(filepath.Join(ModeDir, modesFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	return lines, sc.Err()
}

func modeOf(line string) string {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	p := strings.SplitN(line, "\t", 2)
	if len(p) != 2 {
		return ""
	}
	return strings.TrimSpace(p[0])
}

func modeListed(mode string) bool {
	lines, _ := readModeLines()
	n := 0
	for _, l := range lines {
		if m := modeOf(l); m != "" {
			if m == mode {
				return n < maxModes
			}
			n++
		}
	}
	return false
}

// upsertMode replaces the mode's row (keeps its position) or appends one; refuses a 5th mode.
func upsertMode(mode, display string) error {
	lines, err := readModeLines()
	if err != nil {
		return err
	}
	row, n, found := mode+"\t"+display, 0, false
	for i, l := range lines {
		if m := modeOf(l); m != "" {
			n++
			if m == mode {
				lines[i], found = row, true
			}
		}
	}
	if !found {
		if n >= maxModes {
			return bad("modes.tsv already lists %d modes; manifest installed but not listed", maxModes)
		}
		lines = append(lines, row)
	}
	return writeAtomic(ModeDir, modesFile, []byte(strings.Join(lines, "\n")+"\n"))
}

type Installed struct {
	File    string   `json:"file"`
	Mode    string   `json:"mode"`
	Counter uint32   `json:"counter"`
	Label   string   `json:"label"`
	Bytes   int      `json:"bytes"`
	Parts   []string `json:"parts"`
	Missing []string `json:"missing"` // parts absent or with a wrong hash (the console would fail)
	Listed  bool     `json:"listed"`
}

// List reports every manifest in Dir (<game>.bin and <game>.<name>.bin; dated events included).
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
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, game) || !strings.HasSuffix(n, ".bin") {
			continue
		}
		mode := strings.TrimSuffix(strings.TrimPrefix(n, game), ".bin")
		if mode != "" && (mode[0] != '.' || !validName(mode[1:])) {
			continue
		}
		mode = strings.TrimPrefix(mode, ".")
		data, err := os.ReadFile(filepath.Join(Dir, n))
		if err != nil {
			continue
		}
		it := Installed{File: n, Mode: mode, Bytes: len(data), Parts: []string{}, Missing: []string{}}
		if mode == "" {
			it.Mode, it.Listed = "default", true
		} else {
			it.Listed = modeListed(mode)
		}
		if m, err := Parse(data); err == nil {
			it.Counter, it.Label = m.Counter, m.Label
			for _, p := range m.Parts {
				it.Parts = append(it.Parts, p.Name())
				have, err := os.ReadFile(filepath.Join(Dir, "parts", p.Name()+".bin"))
				if err != nil || !hashOK(p, have) {
					it.Missing = append(it.Missing, p.Name())
				}
			}
		} else {
			it.Label = "INVALID: " + err.Error()
		}
		out = append(out, it)
	}
	return out, nil
}
