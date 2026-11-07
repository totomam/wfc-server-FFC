package nas

import (
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

var dlsActions = map[string]func(moduleName string, fields map[string][]byte) []byte{
	"count":    dlsCount,
	"list":     dlsList,
	"contents": dlsContents,
	"balance":  dlsBalance,
	"package":  dlsPackage,
	"result":   dlsResult,
}

// pkgMaxLen is the client's package rx limit (PKG_MAX_LEN, docs/online/PACKAGE_WIRE.md in ffc-decomp).
const pkgMaxLen = 48 * 1024

func handleDownloadEndpoint(w http.ResponseWriter, r *http.Request) {
	moduleName := getModuleName(r)

	fields, err := parseAuthRequest(r)
	if err != nil {
		replyHTTPError(w, 400, "400 Bad Request")
		return
	}

	action := string(fields["action"])
	if action == "" {
		logging.Error(moduleName, "No action in form")
		replyHTTPError(w, 400, "400 Bad Request")
		return
	}

	rhgamecd, ok := fields["rhgamecd"]
	if !ok || !isValidRHGameCode(string(rhgamecd)) {
		logging.Error(moduleName, "Missing or invalid rhgamecd")
		replyHTTPError(w, 400, "400 Bad Request")
		return
	}

	if actionFunc, exists := dlsActions[strings.ToLower(action)]; exists {
		reply := actionFunc(moduleName, fields)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", strconv.Itoa(len(reply)))
		_, err := w.Write(reply)
		if err != nil {
			logging.Error(moduleName, "Error writing response:", err)
		}
		return
	}

	logging.Error(moduleName, "Unknown action:", aurora.Cyan(action))
	replyHTTPError(w, 400, "400 Bad Request")
}

// dlcFiles lists the servable Bonus Data files in dlc/<rhgamecd>/, sorted by name: regular files only, names of
// 1..32 chars without a leading dot (FFC parses the name with "%d" -> content id 1..64; docs/sessions/s74-online/
// bonus_data.md in the ffc-decomp repo). count, list and contents all use this one view so they always agree.
func dlcFiles(rhgamecd string) []os.DirEntry {
	dir, err := os.ReadDir(filepath.Join(dlcDir, rhgamecd))
	if err != nil {
		return nil
	}
	var files []os.DirEntry
	for _, e := range dir {
		name := e.Name()
		if !e.Type().IsRegular() || len(name) == 0 || len(name) > 32 || name[0] == '.' {
			continue
		}
		files = append(files, e)
	}
	return files
}

func dlsCount(moduleName string, fields map[string][]byte) []byte {
	return append([]byte(strconv.Itoa(len(dlcView(fields)))), 0)
}

// dlsList answers action=list (offset, num): one line per file, "name\tB64(desc)\tattr1\tattr2\tattr3\tsize\r\n".
// The client requires size == the byte length it later gets from action=contents.
func dlsList(moduleName string, fields map[string][]byte) []byte {
	files := dlcView(fields)
	offset, _ := strconv.Atoi(string(fields["offset"]))
	num, err := strconv.Atoi(string(fields["num"]))
	if err != nil || num <= 0 {
		num = len(files)
	}
	if offset < 0 || offset > len(files) {
		offset = len(files)
	}
	end := offset + num
	if end > len(files) {
		end = len(files)
	}
	var out []byte
	for _, e := range files[offset:end] {
		desc := base64.StdEncoding.EncodeToString([]byte("Bonus Data " + e.name))
		out = append(out, []byte(e.name+"\t"+desc+"\t\t\t\t"+strconv.FormatInt(e.size, 10)+"\r\n")...)
	}
	return out
}

// dlsContents answers action=contents (contents = file name): the raw file bytes. Only names that dlcView lists are
// served (no path separators, no traversal).
func dlsContents(moduleName string, fields map[string][]byte) []byte {
	rhgamecd := string(fields["rhgamecd"])
	want := string(fields["contents"])
	for _, e := range dlcView(fields) {
		if e.name == want {
			data, err := e.read(rhgamecd)
			if err != nil {
				return nil
			}
			return data
		}
	}
	logging.Error(moduleName, "Unknown Bonus Data file:", aurora.Cyan(want))
	return nil
}

// dlsBalance serves the wire-format table from docs/online/BALANCE_WIRE.md (ffc-decomp repo) for the "balance"
// Nd action (mods/net_balance, same free-form-action mechanism as "count"/"list"/"contents": src/ov001/setupHttp.c
// POSTs whatever action string the client asks for to this same endpoint). A missing file -> a well-formed EMPTY
// table (count=0, correct CRC), not an error response: the client's BalanceParse must see one well-formed shape or
// the other, never a malformed one it has to special-case.
func dlsBalance(moduleName string, fields map[string][]byte) []byte {
	path := filepath.Join(balanceDir, string(fields["rhgamecd"])+".bin")
	data, err := os.ReadFile(path)
	if err != nil {
		return emptyBalanceTable()
	}
	return data
}

// dlsPackage serves the SIGNED code package (docs/online/PACKAGE_WIRE.md, ffc-decomp; built + signed offline by
// tools/online_pkg.py on the key holder's PC) for the "package" Nd action. The server never signs or checks the
// signature: the client verifies Ed25519 itself. No file (or one over the client's limit) -> "NONE", which fails
// the client's PkgCheck = vanilla rules (stock bytes), the same as a stock server.
func dlsPackage(moduleName string, fields map[string][]byte) []byte {
	path := filepath.Join(packageDir, string(fields["rhgamecd"])+".bin")
	data, err := os.ReadFile(path)
	if err != nil || len(data) > pkgMaxLen {
		if err == nil {
			logging.Error(moduleName, "package too large:", aurora.Cyan(len(data)))
		}
		return []byte("NONE")
	}
	return data
}

func emptyBalanceTable() []byte {
	buf := make([]byte, 16) // 12-byte header + 0 records + 4-byte CRC32, count == 0
	copy(buf[0:4], "FFCB")
	buf[4] = 1 // version
	binary.LittleEndian.PutUint16(buf[8:10], 0)
	crc := crc32.ChecksumIEEE(buf[0:12])
	binary.LittleEndian.PutUint32(buf[12:16], crc)
	return buf
}

func isValidRHGameCode(rhgamecd string) bool {
	if len(rhgamecd) != 4 {
		return false
	}

	return common.IsUppercaseAlphanumeric(rhgamecd)
}
