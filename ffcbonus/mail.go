package ffcbonus

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Per-player mail inbox (ffc-decomp ROADMAP #77, s104-online; docs/online/NEWS.md "Mail"). Each player has up to
// MailSlots messages in Dir/mail/<gamecd>/<pid>/<slot>. They are served to that player's console only, as Bonus Data
// files named MailName(slot) = "165".."180": the game keeps id % 100 (65..80), which the modded package gate shows on
// every visit like news, so a game running the package lists the inbox each time; a stock game skips them (> 64).
// pid = the player's MAIN profile (admin links #53 resolved by the api package).

const (
	MailSlots    = 16 // 3 generated + 16 mail + 32 news <= the 64 list entries the game reads
	mailNameBase = 164
)

var ErrSlot = errors.New("slot must be 1..16 (0 = first free slot)")
var ErrFull = errors.New("inbox full (16 messages): delete one first")

type Mail struct {
	Slot int    `json:"slot"`
	Text string `json:"text"`
}

// MailName: the Bonus Data file name of a mail slot ("165".."180").
func MailName(slot int) string { return strconv.Itoa(mailNameBase + slot) }

// IsMailName: name is a mail slot file name.
func IsMailName(name string) bool {
	n, err := strconv.Atoi(name)
	return err == nil && strconv.Itoa(n) == name && n > mailNameBase && n <= mailNameBase+MailSlots
}

func mailDir(gamecd string, pid uint32) string {
	return filepath.Join(Dir, "mail", gamecd, strconv.FormatUint(uint64(pid), 10))
}

// MailList returns one player's inbox, by slot.
func MailList(gamecd string, pid uint32) ([]Mail, error) {
	if !validCode(gamecd) {
		return nil, ErrCode
	}
	ents, err := os.ReadDir(mailDir(gamecd, pid))
	if errors.Is(err, os.ErrNotExist) {
		return []Mail{}, nil
	} else if err != nil {
		return nil, err
	}
	out := []Mail{}
	for _, e := range ents {
		s, err := strconv.Atoi(e.Name())
		if err != nil || !e.Type().IsRegular() || strconv.Itoa(s) != e.Name() || s < 1 || s > MailSlots {
			continue
		}
		b, err := os.ReadFile(filepath.Join(mailDir(gamecd, pid), e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, Mail{Slot: s, Text: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out, nil
}

// MailSet writes slot (1..16; 0 = the first free slot) and returns the slot used.
func MailSet(gamecd string, pid uint32, slot int, text string) (int, error) {
	if !validCode(gamecd) {
		return 0, ErrCode
	}
	if slot < 0 || slot > MailSlots {
		return 0, ErrSlot
	}
	if !ValidText(text) || len(text) > MaxText-mailHeaderMax {
		return 0, ErrText
	}
	if slot == 0 {
		ms, err := MailList(gamecd, pid)
		if err != nil {
			return 0, err
		}
		used := map[int]bool{}
		for _, m := range ms {
			used[m.Slot] = true
		}
		for s := 1; s <= MailSlots && slot == 0; s++ {
			if !used[s] {
				slot = s
			}
		}
		if slot == 0 {
			return 0, ErrFull
		}
	}
	return slot, writeAtomic(mailDir(gamecd, pid), strconv.Itoa(slot), text)
}

// MailDelete removes one slot (no error when empty); MailClear empties the inbox.
func MailDelete(gamecd string, pid uint32, slot int) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	if slot < 1 || slot > MailSlots {
		return ErrSlot
	}
	err := os.Remove(filepath.Join(mailDir(gamecd, pid), strconv.Itoa(slot)))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func MailClear(gamecd string, pid uint32) error {
	if !validCode(gamecd) {
		return ErrCode
	}
	return os.RemoveAll(mailDir(gamecd, pid))
}

// mailHeaderMax: room kept for the "MAIL nn/nn\n" header in the 0x3ff game buffer.
const mailHeaderMax = 12

// MailItems: the inbox as Bonus Data items (name MailName(slot), body "MAIL i/n\n" + text), for the NAS news view.
func MailItems(gamecd string, pid uint32) []NewsItem {
	ms, err := MailList(gamecd, pid)
	if err != nil || len(ms) == 0 {
		return nil
	}
	out := make([]NewsItem, 0, len(ms))
	for i, m := range ms {
		body := "MAIL " + strconv.Itoa(i+1) + "/" + strconv.Itoa(len(ms)) + "\n" + m.Text
		if len(body) > MaxText {
			body = body[:MaxText]
		}
		out = append(out, NewsItem{Name: MailName(m.Slot), Body: []byte(body)})
	}
	return out
}

// writeAtomic: temp file + rename, so a download never sees half a file.
func writeAtomic(dir, name, text string) error {
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
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
