package nas

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
	"wwfc/ffcbonus"
)

// In-game news (ffc-decomp ROADMAP #76/#73/#79/#41, s103-online; docs/online/NEWS.md in ffc-decomp). The game's Bonus
// Data box shows each listed file's body as text. A stock game only shows ids 1..64 once (save flag); a game running
// the modded package (ov007 sites) also shows ids 65..99 on EVERY visit. 65..96 are admin files (ffcbonus); 97..99 are
// generated per player by ffcbonus.NewsSource (set by the api package): profile, top players, challenges; 165..180
// = that player's mail inbox (#77, s104-online; the game keeps id % 100 = 65..80, shown like news).

// newsTTL keeps one console's generated items fixed between its list and contents requests: the client needs
// contents length == the size given by list (a result arriving in between must not change the text).
const newsTTL = 60 * time.Second

type newsEntry struct {
	at    time.Time
	items []ffcbonus.NewsItem
}

var (
	newsMu    sync.Mutex
	newsCache = map[string]newsEntry{}
	newsNow   = time.Now
)

func newsItems(gamecd string, fields map[string][]byte) []ffcbonus.NewsItem {
	if ffcbonus.NewsSource == nil {
		return nil
	}
	userID, err := strconv.ParseUint(string(fields["userid"]), 10, 64)
	if err != nil || userID == 0 {
		return nil
	}
	key := gamecd + "/" + strconv.FormatUint(userID, 10)
	now := newsNow()
	newsMu.Lock()
	if e, ok := newsCache[key]; ok && now.Sub(e.at) < newsTTL {
		newsMu.Unlock()
		return e.items
	}
	for k, e := range newsCache { // small map: drop expired entries on each miss
		if now.Sub(e.at) >= newsTTL {
			delete(newsCache, k)
		}
	}
	newsMu.Unlock()
	var items []ffcbonus.NewsItem
	for _, it := range ffcbonus.NewsSource(gamecd, userID) {
		if n, err := strconv.Atoi(it.Name); err != nil || (n < 97 || n > 99) && !ffcbonus.IsMailName(it.Name) || len(it.Body) == 0 || len(it.Body) > 0x3ff {
			continue
		}
		items = append(items, it)
	}
	newsMu.Lock()
	newsCache[key] = newsEntry{now, items}
	newsMu.Unlock()
	return items
}

// dlcEntry is one servable Bonus Data file (on disk or generated).
type dlcEntry struct {
	name string
	size int64
	data []byte // generated item; nil = read dlc/<gamecd>/<name>
}

// dlcView is what count, list and contents all see for one request: generated news FIRST (the client handles at most
// 64 list entries, so news is never cut by many messages), then the files of dlc/<gamecd>/ (dlcFiles). A file named
// like a generated item is hidden while that item exists.
func dlcView(fields map[string][]byte) []dlcEntry {
	gamecd := string(fields["rhgamecd"])
	var out []dlcEntry
	gen := map[string]bool{}
	for _, it := range newsItems(gamecd, fields) {
		out = append(out, dlcEntry{it.Name, int64(len(it.Body)), it.Body})
		gen[it.Name] = true
	}
	for _, e := range dlcFiles(gamecd) {
		if gen[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, dlcEntry{name: e.Name(), size: info.Size()})
	}
	return out
}

func (e dlcEntry) read(gamecd string) ([]byte, error) {
	if e.data != nil {
		return e.data, nil
	}
	return os.ReadFile(filepath.Join(dlcDir, gamecd, e.name))
}
