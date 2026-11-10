package gpcm

import (
	"strings"
	"unicode/utf8"
)

// Fossil Fighters: Champions (dental2ds) status caps (FFC decomp NET-39, server half of NET-38/NET-04).
//
// The game copies status values into tiny stack buffers without a bound:
//   - DWC_GetCommonValueString copies a /KEY/value value into buf[4]
//     (DWCi_IsLockedBuddyStatus, DWC_ConnectToGameServerAsync), so a value of
//     12+ chars reaches the saved return address. The honest client only
//     writes /SCM/, /SCN/, /VER/90 and /LCK/ with small decimals.
//   - the friend's name from the locstring "MO_<digit>_NAME_<name>" is
//     wcscpy'd into a 9-halfword field, so the name may hold at most 8 chars.
// The server refuses such a status instead of relaying it to other players.

const ffcGameName = "dental2ds"

const (
	ffcMaxStatusValueLen = 3
	ffcMaxLocNameLen     = 8
)

// ffcStatStringValid reports whether every value in a "/KEY/value/KEY/value"
// status string is at most 3 characters long.
func ffcStatStringValid(statstring string) bool {
	if statstring == "" {
		return true
	}
	if !strings.HasPrefix(statstring, "/") {
		return false
	}
	parts := strings.Split(statstring[1:], "/")
	for i := 1; i < len(parts); i += 2 {
		if len(parts[i]) > ffcMaxStatusValueLen {
			return false
		}
	}
	return true
}

// ffcLocStringValid reports whether locstring is empty or
// "MO_<digit>_NAME_<at most 8 chars>".
func ffcLocStringValid(locstring string) bool {
	if locstring == "" {
		return true
	}
	if len(locstring) < 3+1+6 || !strings.HasPrefix(locstring, "MO_") {
		return false
	}
	if c := locstring[3]; c < '0' || c > '9' {
		return false
	}
	name, ok := strings.CutPrefix(locstring[4:], "_NAME_")
	if !ok || !utf8.ValidString(name) {
		return false
	}
	return utf8.RuneCountInString(name) <= ffcMaxLocNameLen
}

// ffcStatusValid applies both checks.
func ffcStatusValid(statstring, locstring string) bool {
	return ffcStatStringValid(statstring) && ffcLocStringValid(locstring)
}

// ffcRelayAllowed: a status may reach a dental2ds player only if it passes
// the caps (second guard, in case the sender is not itself a dental2ds session).
func ffcRelayAllowed(receiver, sender *GameSpySession) bool {
	if receiver.GameName != ffcGameName && sender.GameName != ffcGameName {
		return true
	}
	return ffcStatusValid(sender.StatString, sender.LocString)
}
