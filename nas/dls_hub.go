package nas

import (
	"strconv"
	"strings"
	"wwfc/ffcbonus"
)

// FFC Online hub (ffc-decomp s113-online): the modded ROM's "Online" choice window asks Nd action "hub" with attr1 =
// one section and shows the answer as message-box pages separated by '\f' (one box per page):
//   top  -> 98 top players
//   me   -> 97 your ranked stats, 196 your last matches, 99 challenges
//   mail -> your mail (165..180) then admin news (dlc files 65..96)
// Same texts as the Bonus Data news (newsItems, dlcView); an empty section answers one "nothing yet" page.

const hubMax = 2048 // client buffer (net_balance_net.c NB_HUB_MAX)

func hubPages(section string, gen []ffcbonus.NewsItem, files []dlcEntry, gamecd string) []string {
	byName := map[string]string{}
	var mail []string
	for _, it := range gen {
		if ffcbonus.IsMailName(it.Name) {
			mail = append(mail, string(it.Body))
		} else {
			byName[it.Name] = string(it.Body)
		}
	}
	pick := func(names ...string) []string {
		var out []string
		for _, n := range names {
			if s := byName[n]; s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	var pages []string
	empty := ""
	switch section {
	case "top":
		pages, empty = pick("98"), "TOP PLAYERS\nNo ranking yet: a player\nneeds 5 ranked matches."
	case "me":
		pages, empty = pick("97", "196", "99"), "YOUR STATS\nNo ranked matches yet.\nTry Battle with Anyone!"
	case "mail":
		pages = mail
		for _, e := range files {
			if n, err := strconv.Atoi(e.name); err == nil && n >= 65 && n <= 96 && e.data == nil {
				if b, err := e.read(gamecd); err == nil && len(b) > 0 {
					pages = append(pages, string(b))
				}
			}
		}
		empty = "MAIL & NEWS\nNothing new."
	default:
		return nil
	}
	if len(pages) == 0 {
		pages = []string{empty}
	}
	return pages
}

func dlsHub(moduleName string, fields map[string][]byte) []byte {
	gamecd := string(fields["rhgamecd"])
	pages := hubPages(string(fields["attr1"]), newsItems(gamecd, fields), dlcView(fields), gamecd)
	if pages == nil {
		return []byte("err")
	}
	out := ""
	for _, p := range pages {
		p = strings.ReplaceAll(strings.TrimRight(p, "\n"), "\f", "\n")
		if len(out)+len(p)+1 > hubMax {
			break
		}
		if out != "" {
			out += "\f"
		}
		out += p
	}
	return []byte(out)
}
