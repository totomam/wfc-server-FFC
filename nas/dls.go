package nas

import (
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
	"count":   dlsCount,
	"balance": dlsBalance,
	"package": dlsPackage,
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

func dlsCount(moduleName string, fields map[string][]byte) []byte {
	dlcFolder := filepath.Join(dlcDir, string(fields["rhgamecd"]))

	dir, err := os.ReadDir(dlcFolder)
	if err != nil {
		return []byte{'0', 0}
	}

	return append([]byte(strconv.Itoa(len(dir))), 0)
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
