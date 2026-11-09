package api

import (
	"errors"
	"net/http"
	"wwfc/ffctable"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC live balance overlay (FFCO v1, s129-online), admin only:
//   GET  /api/ffc/admin/table?secret=…   -> {"tables": [{file, mode, rows, serial, crc, bytes}]}
//   POST /api/ffc/admin/table {"secret", "mode", "table": b64}
// mode "" / "default" = the base overlay <dir>/VDEE.ffco, else VDEE.<mode>.ffco. Format checked here only (magic,
// version, n, length, crc, file < 2); an empty table removes the overlay. Live at once: the Nd action "table" serves it.

const ffcTableBodyMax = 64 << 10

type ffcTableRequest struct {
	AuthInfo
	Mode  string `json:"mode"`
	Table []byte `json:"table"`
}

func HandleFFCTable(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		list, err := ffctable.List(ffcBonusGame)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "table_error")
			return
		}
		replyOK(w, map[string]any{"tables": list})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, ffcTableBodyMax)
	var req ffcTableRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	err := ffctable.Install(ffcBonusGame, req.Mode, req.Table)
	switch {
	case errors.Is(err, ffctable.ErrTable):
		replyError(w, http.StatusBadRequest, APIErrorString(err.Error()))
		return
	case err != nil:
		logging.Error("API", "table upload:", err.Error())
		replyError(w, http.StatusInternalServerError, "table_error")
		return
	}
	logging.Notice("API", "table upload", aurora.Cyan(req.Mode), "bytes", aurora.Cyan(len(req.Table)))
	replyOK(w, map[string]any{"ok": true, "bytes": len(req.Table)})
}
