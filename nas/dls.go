package nas

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"wwfc/common"
	"wwfc/ffcbonus"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

var dlsActions = map[string]func(moduleName string, fields map[string][]byte) []byte{
	"count":    dlsCount,
	"list":     dlsList,
	"contents": dlsContents,
	"result":   dlsResult,
	"motd":     dlsMotd,
	"modes":    dlsModes,
	"lbname":   dlsLbName,
	"hub":      dlsHub,
	"boot":     dlsBoot,
	"bootpart": dlsBootPart,
	"table":    dlsTable,
}

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
	if offset < 0 || (err == nil && num < 0) {
		logging.Error(moduleName, "Negative list offset/num:", aurora.Cyan(string(fields["offset"])), aurora.Cyan(string(fields["num"])))
		return nil
	}
	if err != nil || num == 0 {
		num = len(files)
	}
	if offset > len(files) {
		offset = len(files)
	}
	// Clamp before adding: offset+num must not overflow (num=MaxInt made end negative)
	if num > len(files)-offset {
		num = len(files) - offset
	}
	end := offset + num
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

// dlsMotd serves the login MOTD (ffc-decomp ROADMAP #54, s105-online; ffcbonus/motd.go) for the "motd" Nd action
// of the modded loader. No text -> an empty body (the client shows no box). Only the loader asks: stock games never.
func dlsMotd(moduleName string, fields map[string][]byte) []byte {
	s, err := ffcbonus.Motd(string(fields["rhgamecd"]))
	if err != nil {
		return []byte{}
	}
	return []byte(s)
}

func isValidRHGameCode(rhgamecd string) bool {
	if len(rhgamecd) != 4 {
		return false
	}

	return common.IsUppercaseAlphanumeric(rhgamecd)
}
