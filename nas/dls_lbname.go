package nas

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC in-game leaderboard name (ffc-decomp ROADMAP #52, s113-online; client mods/net_balance net_balance_net.c
// kind 5 + net_lbname.c). Nd action "lbname":
//   no attr1 -> get: "1" = one of this console's FFC profiles has a name (copied to its other profiles, e.g. a new
//               save), "0" = not yet (the client then opens the stock name-entry keyboard once)
//   attr1    -> set: attr1 = the UTF-8 name (DWC-base64, decoded by parseAuthRequest); "ok", "bad" (refused: empty,
//               too long, control characters, profanity.txt word) or "err" (no user id / database)
// The request carries only the console's NAS user id: the name goes to every FFC profile of that user id.

const lbNameMaxRunes = 16

type lbNameStore interface {
	ProfilesOfUser(userID uint64, gamecd string) ([]uint32, error)
	LbNameOf(pids []uint32) (string, []uint32, error)
	SetLbName(pids []uint32, name string) error
}

var lbNameDB lbNameStore

// lbNameIsBad is IsBadWord (profanity.txt); a test replaces it.
var lbNameIsBad = IsBadWord

// cleanLbName trims the name and checks it; "" = refused.
func cleanLbName(raw []byte) string {
	if !utf8.Valid(raw) {
		return ""
	}
	s := strings.Join(strings.Fields(string(raw)), " ")
	if s == "" || utf8.RuneCountInString(s) > lbNameMaxRunes {
		return ""
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	words := append(strings.Fields(s), strings.ReplaceAll(s, " ", ""))
	for _, w := range words {
		if bad, err := lbNameIsBad(w); err == nil && bad {
			return ""
		}
	}
	return s
}

func dlsLbName(moduleName string, fields map[string][]byte) []byte {
	userID, err := strconv.ParseUint(string(fields["userid"]), 10, 64)
	if err != nil || userID == 0 || lbNameDB == nil {
		return []byte("err")
	}
	pids, err := lbNameDB.ProfilesOfUser(userID, string(fields["rhgamecd"]))
	if err != nil || len(pids) == 0 {
		logging.Error(moduleName, "lbname: no profile for user", aurora.Cyan(userID))
		return []byte("err")
	}
	raw, set := fields["attr1"]
	if !set || len(raw) == 0 {
		name, missing, err := lbNameDB.LbNameOf(pids)
		if err != nil {
			return []byte("err")
		}
		if name == "" {
			return []byte("0")
		}
		if len(missing) > 0 { // a new save / profile on a named console: same name
			if err := lbNameDB.SetLbName(missing, name); err != nil {
				logging.Error(moduleName, "lbname:", err)
			}
		}
		return []byte("1")
	}
	name := cleanLbName(raw)
	if name == "" {
		logging.Notice(moduleName, "lbname: refused name from user", aurora.Cyan(userID))
		return []byte("bad")
	}
	if err := lbNameDB.SetLbName(pids, name); err != nil {
		logging.Error(moduleName, "lbname:", err)
		return []byte("err")
	}
	logging.Notice(moduleName, "lbname: user", aurora.Cyan(userID), "profiles", aurora.Cyan(pids), "name", aurora.Cyan(name))
	return []byte("ok")
}
