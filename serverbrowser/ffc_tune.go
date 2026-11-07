package serverbrowser

import (
	"wwfc/ffcmatch"
	"wwfc/logging"
	"wwfc/qr2"

	"github.com/logrusorgru/aurora/v3"
)

// ffcTune applies the FFC Battle with Anyone tuning (package ffcmatch, ROADMAP #34) to an already-filtered list.
func ffcTune(moduleName string, all, servers []map[string]string, filter string) []map[string]string {
	pid, ok := ffcmatch.SearcherPid(filter)
	if !ok {
		return servers
	}
	var self map[string]string
	for _, s := range all {
		if s["gamename"] == ffcmatch.GameName && s["dwc_pid"] == pid {
			self = s
			break
		}
	}
	out, elapsed := ffcmatch.Apply(pid, self, servers, qr2.GameCodeOf)
	if len(out) != len(servers) {
		logging.Info(moduleName, "FFC match tuning:", aurora.Cyan(pid), "kept", aurora.BrightCyan(len(out)), "of",
			aurora.BrightCyan(len(servers)), "after", aurora.Cyan(elapsed.Round(1e9).String()))
	}
	return out
}
