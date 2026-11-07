// Package ffcbonus manages FFC Bonus Data messages on disk (ffc-decomp ROADMAP #96 custom messages, #38
// announcements). The NAS serves dlc/<gamecd>/<file> via Nd count/list/contents (nas/dls.go); the game parses the
// file name as a decimal content id 1..64, shows the file body as TEXT in a message box (cut at 0x3ff chars) and sets
// a per-save "received" flag for that id, so each id reaches a save ONCE. Ids 1..7 also give fossil rocks 600..606
// (empty records in the USA ROM), ids 8..64 map to rock id 0 (effect not verified in game; ffc-decomp
// docs/sessions/s74-online/bonus_data.md).
//
// s103-online (#76 news): ids 65..96 are NEWS slots. A stock game skips them (its id check is 1..64); a game running
// the modded package (ffc-decomp docs/online/NEWS.md, ov007 sites) shows them on EVERY Bonus Data visit, with no
// flag and no rock. Ids 97..99 are generated per player by the server (nas NewsSource) and cannot be written here.
package ffcbonus

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Dir is the Bonus Data root, the same folder the NAS reads (nas dlcDir, "./dlc").
var Dir = "./dlc"

const (
	MaxText = 0x3ff // the game's message buffer
	MinID   = 1
	MaxID   = 96 // 1..64 one-shot messages, 65..96 news (NewsMin)
	NewsMin = 65
)

var (
	ErrID   = errors.New("id must be 1..64 (message) or 65..96 (news)")
	ErrText = errors.New("text must be 1..1023 printable ASCII characters (newlines allowed)")
	ErrCode = errors.New("bad game code")
)

// NewsItem is one generated Bonus Data file: Name = decimal id 97..99, Body = ASCII text (<= 0x3ff bytes).
type NewsItem struct {
	Name string
	Body []byte
}

// NewsSource returns the generated items for one console (the Nd userid field), or nil; set by the api package
// (ffc_news.go), read by the NAS (nas/dls_news.go). nil = no generated news.
var NewsSource func(gamecd string, userID uint64) []NewsItem

// BadgeSource returns a profile's Plaza ghost badge ("" = none; #78, read by sake/ffc_badge.go, set by the api
// package ffc_badge.go). nil = no badges.
var BadgeSource func(ownerID uint32) string

type Message struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

func validCode(gamecd string) bool {
	if len(gamecd) != 4 {
		return false
	}
	for _, c := range gamecd {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// ValidText: what the DS font shows safely (s74: ASCII displays fine).
func ValidText(s string) bool {
	if len(s) == 0 || len(s) > MaxText {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\n' && (c < 0x20 || c > 0x7e) {
			return false
		}
	}
	return true
}

// List returns the numeric-named messages (other files in the folder are left alone and not listed).
func List(gamecd string) ([]Message, error) {
	if !validCode(gamecd) {
		return nil, ErrCode
	}
	ents, err := os.ReadDir(filepath.Join(Dir, gamecd))
	if errors.Is(err, os.ErrNotExist) {
		return []Message{}, nil
	} else if err != nil {
		return nil, err
	}
	out := []Message{}
	for _, e := range ents {
		id, err := strconv.Atoi(e.Name())
		if err != nil || !e.Type().IsRegular() || strconv.Itoa(id) != e.Name() || id < MinID || id > MaxID {
			continue
		}
		b, err := os.ReadFile(filepath.Join(Dir, gamecd, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, Message{ID: id, Text: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Set writes (or replaces) message id atomically (temp file + rename, so a download never sees half a file).
func Set(gamecd string, id int, text string) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	if id < MinID || id > MaxID {
		return ErrID
	}
	if !ValidText(text) {
		return ErrText
	}
	dir := filepath.Join(Dir, gamecd)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".msg-*")
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
		err = os.Rename(tmp.Name(), filepath.Join(dir, strconv.Itoa(id)))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}

// Delete removes message id (no error when it does not exist).
func Delete(gamecd string, id int) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	if id < MinID || id > MaxID {
		return ErrID
	}
	err := os.Remove(filepath.Join(Dir, gamecd, strconv.Itoa(id)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
