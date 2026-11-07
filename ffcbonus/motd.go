package ffcbonus

import (
	"errors"
	"os"
	"path/filepath"
)

// Login MOTD (ffc-decomp ROADMAP #54, s105-online): one admin text per game, shown by the modded loader
// (mods/net_balance) in the game's OK message box once per Wi-Fi connection, right at the Wi-Fi top menu. Stored as
// <Dir>/<gamecd>.motd, outside dlc/<gamecd>/ so it is never listed as a Bonus Data file. The client fetches it with
// the Nd action "motd" (nas/dls.go); empty/missing = no box. Stock games never ask.

// MotdMax: the box shows ~10 rows of ~36 chars (s104-online measurement); the client keeps at most 0x17f bytes.
const MotdMax = 360

var ErrMotd = errors.New("motd must be 1..360 printable ASCII characters (newlines allowed)")

func motdPath(gamecd string) string { return filepath.Join(Dir, gamecd+".motd") }

// Motd returns the current text ("" = none).
func Motd(gamecd string) (string, error) {
	if !validCode(gamecd) {
		return "", ErrCode
	}
	b, err := os.ReadFile(motdPath(gamecd))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(b) > MotdMax || !ValidText(string(b)) {
		return "", nil // hand-edited file out of bounds: show nothing rather than a cut text
	}
	return string(b), nil
}

// SetMotd writes the text atomically (temp file + rename).
func SetMotd(gamecd, text string) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	if len(text) > MotdMax || !ValidText(text) {
		return ErrMotd
	}
	if err := os.MkdirAll(Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(Dir, ".motd-*")
	if err != nil {
		return err
	}
	if _, err = tmp.WriteString(text); err == nil {
		err = tmp.Close()
	} else {
		_ = tmp.Close()
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), motdPath(gamecd))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// DeleteMotd removes the text (no error if there is none).
func DeleteMotd(gamecd string) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	err := os.Remove(motdPath(gamecd))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
