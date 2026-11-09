package nas

import (
	"os"
	"wwfc/ffctable"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// Live balance overlay (ffc-decomp s126/s129-online): Nd action "table", attr1 = mode (modes.tsv; ""/"default" = the
// base file). Returns the FFCO v1 file <balanceDir>/<game>[.<mode>].ffco byte for byte; "NONE" = no overlay
// (unknown / unlisted mode, no file, or an invalid file). The console re-checks every row; unsigned data.
// tableNone: "no overlay". An EMPTY Nd body counts as a failed request on the console (stage-0 done(ok=0)), which
// would keep the previous overlay, so removal / no file answers this marker; the console refuses it as an overlay.
var tableNone = []byte("NONE")

func dlsTable(moduleName string, fields map[string][]byte) []byte {
	mode := string(fields["attr1"])
	if mode != "" && mode != "default" {
		listed := false
		for _, m := range readModes() {
			if m.Name == mode {
				listed = true
				break
			}
		}
		if !listed {
			return tableNone
		}
	}
	path, err := ffctable.File(string(fields["rhgamecd"]), mode)
	if err != nil {
		return tableNone
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return tableNone
	}
	if _, _, _, err := ffctable.Validate(data); err != nil {
		logging.Error(moduleName, "table invalid:", aurora.Cyan(path), err)
		return tableNone
	}
	return data
}
