package nas

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// Stage-0 loader (ffc-decomp docs/online/STAGE0.md, s116-online): the frozen ROM patch asks "boot" on every Wi-Fi
// top menu visit for a SIGNED manifest (built + signed offline by tools/online_boot.py; the server never signs or
// checks it, the console does), then fetches each part with "bootpart" in 16 KB chunks. Parts are content-addressed
// (boot/parts/<hex16>.bin, hex16 = first 8 SHA-512 bytes), so every mode/event manifest can share them and an
// upload never races a download in progress. Stock games never ask.
var bootDir = "./boot"

const (
	bootManifestMax = 16 * 1024 // S0_STAGING
	bootChunk       = 16 * 1024 // S0_CHUNK
)

var bootPartName = regexp.MustCompile(`^[0-9a-f]{16}$`)

// dlsBoot: manifest for the request's mode (attr1, modes.tsv) / dated event / base file, else "NONE" (= the console
// stays stock).
func dlsBoot(moduleName string, fields map[string][]byte) []byte {
	path := modeFile(bootDir, string(fields["rhgamecd"]), fields, time.Now())
	data, err := os.ReadFile(path)
	if err != nil || len(data) > bootManifestMax || len(data) < 4 || string(data[:4]) != "FFCM" {
		if err == nil {
			logging.Error(moduleName, "boot manifest invalid:", aurora.Cyan(path))
		}
		return []byte("NONE")
	}
	return data
}

// dlsBootPart: chunk attr2 of part attr3 (hex16). Anything else -> "NONE" (wrong length = the console refuses it).
func dlsBootPart(moduleName string, fields map[string][]byte) []byte {
	name := string(fields["attr3"])
	chunk, err := strconv.Atoi(string(fields["attr2"]))
	if !bootPartName.MatchString(name) || err != nil || chunk < 0 || chunk > 4096 {
		return []byte("NONE")
	}
	data, err := os.ReadFile(filepath.Join(bootDir, "parts", name+".bin"))
	if err != nil {
		logging.Error(moduleName, "boot part missing:", aurora.Cyan(name))
		return []byte("NONE")
	}
	lo := chunk * bootChunk
	if lo >= len(data) {
		return []byte("NONE")
	}
	hi := min(lo+bootChunk, len(data))
	return data[lo:hi]
}
