package api

import (
	"errors"
	"net/http"
	"wwfc/ffcboot"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC stage-0 package upload (ffc-decomp docs/online/STAGE0.md phase 4, s125-online), admin only:
//   GET  /api/ffc/admin/boot?secret=…   -> {"manifests": [{file, mode, counter, label, bytes, parts, missing, listed}]}
//   POST /api/ffc/admin/boot {"secret", "mode", "display", "manifest": b64, "parts": [b64...], "force"}
// The manifest is signed offline (tools/online_pkgnet.py / online_boot.py, key on the admin's PC); the server checks
// format, bounds and every part's length + SHA-512 against it (never the signature), then writes
// boot/parts/<hex16>.bin first and boot/VDEE[.<mode>].bin last (atomic renames). mode "" / "default" = the base
// manifest; display = the Game Mode list name (modes.tsv row added/updated). Parts already on the server may be left
// out. A counter <= the installed manifest's is refused unless force (consoles holding it would refuse: REPLAY).

const ffcBootBodyMax = 1 << 20 // 175.5 KB window + manifest, base64 + JSON

type ffcBootRequest struct {
	AuthInfo
	Mode     string   `json:"mode"`
	Display  string   `json:"display"`
	Manifest []byte   `json:"manifest"`
	Parts    [][]byte `json:"parts"`
	Force    bool     `json:"force"`
}

func HandleFFCBoot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		list, err := ffcboot.List(ffcBonusGame)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "boot_error")
			return
		}
		replyOK(w, map[string]any{"manifests": list})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ffcBootBodyMax)
	var req ffcBootRequest
	if err := parsePostLimit(r, w, &req, RoleModerator, ffcBootBodyMax); err != nil {
		return
	}
	res, err := ffcboot.Install(ffcBonusGame, req.Mode, req.Display, req.Manifest, req.Parts, req.Force)
	switch {
	case errors.Is(err, ffcboot.ErrBoot) && res != nil:
		replyError(w, http.StatusConflict, APIErrorString(err.Error())) // installed, list row refused
		return
	case errors.Is(err, ffcboot.ErrBoot):
		replyError(w, http.StatusBadRequest, APIErrorString(err.Error()))
		return
	case err != nil:
		logging.Error("API", "boot upload:", err.Error())
		replyError(w, http.StatusInternalServerError, "boot_error")
		return
	}
	logging.Notice("API", "boot upload", aurora.Cyan(res.File), "counter", aurora.Cyan(res.Counter),
		"parts written", aurora.Cyan(res.PartsWritten))
	replyOK(w, res)
}
