package nas

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rule-set modes (ffc-decomp ROADMAP #44, s106-online). package/modes.tsv lists "<mode>\t<display name>" (<= 4
// lines; mode "default" = the base files <game>.bin, else a plain name). The modded loader fetches the list (Nd action
// "modes"), asks the player per mode at Battle with Anyone / with a Friend, and re-fetches "package" + "balance" with
// attr1 = <mode> when the pick differs from what it holds: served from <dir>/<game>.<mode>.bin (missing -> the base
// file). Matchmaking already splits players by table/package crc (dwc_mver), so modes never meet.
var modesFile = "modes.tsv"

const maxModes = 4

type ruleMode struct{ Name, Display string }

func validDisplay(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func readModes() []ruleMode {
	f, err := os.Open(filepath.Join(packageDir, modesFile))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []ruleMode
	sc := bufio.NewScanner(f)
	for sc.Scan() && len(out) < maxModes {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.SplitN(line, "\t", 2)
		if len(p) != 2 {
			continue
		}
		name, disp := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
		if !validEventName(name) || !validDisplay(disp) {
			continue
		}
		out = append(out, ruleMode{name, disp})
	}
	return out
}

// dlsModes: "<mode>\t<display>\n" per mode; empty = one rule set (the loader then asks the s89 Yes/No only).
func dlsModes(moduleName string, fields map[string][]byte) []byte {
	var b strings.Builder
	for _, m := range readModes() {
		b.WriteString(m.Name + "\t" + m.Display + "\n")
	}
	return []byte(b.String())
}

// modeFile picks the package/table file for the request's attr1 mode (listed in modes.tsv), else the dated event
// override / base file (activeEventFile).
func modeFile(dir, game string, fields map[string][]byte, now time.Time) string {
	mode := string(fields["attr1"])
	if mode != "" && mode != "default" {
		for _, m := range readModes() {
			if m.Name == mode {
				p := filepath.Join(dir, game+"."+mode+".bin")
				if _, err := os.Stat(p); err == nil {
					return p
				}
				break
			}
		}
	}
	return activeEventFile(dir, game, now)
}
