package api

import (
	"errors"
	"net/http"
	"wwfc/ffcbonus"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC login MOTD (ffc-decomp ROADMAP #54, s105-online), admin only:
//   GET  /api/ffc/admin/motd?secret=…               -> {"text": "..."} ("" = none)
//   POST /api/ffc/admin/motd {"secret","text"}        set/replace (ASCII, <= 360 chars, ~10 rows x 36)
//   POST /api/ffc/admin/motd {"secret","delete":true} remove
// Players running the modded loader see it in an OK box at the Wi-Fi top menu, once per Wi-Fi connection (again
// when the text changes). Stock games never see it.

type ffcMotdRequest struct {
	AuthInfo
	Text   string `json:"text"`
	Delete bool   `json:"delete"`
}

func HandleFFCMotd(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		s, err := ffcbonus.Motd(ffcBonusGame)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "motd_error")
			return
		}
		replyOK(w, map[string]any{"text": s})
		return
	}
	var req ffcMotdRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	var err error
	if req.Delete {
		err = ffcbonus.DeleteMotd(ffcBonusGame)
	} else {
		err = ffcbonus.SetMotd(ffcBonusGame, req.Text)
	}
	switch {
	case errors.Is(err, ffcbonus.ErrMotd):
		replyError(w, http.StatusBadRequest, APIErrorString(err.Error()))
		return
	case err != nil:
		replyError(w, http.StatusInternalServerError, "motd_error")
		return
	}
	logging.Notice("API", "MOTD", aurora.Cyan(map[bool]string{true: "deleted", false: "set"}[req.Delete]))
	replyOK(w, map[string]any{"deleted": req.Delete})
}
