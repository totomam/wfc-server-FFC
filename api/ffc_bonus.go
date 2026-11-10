package api

import (
	"errors"
	"net/http"
	"wwfc/ffcbonus"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC Bonus Data messages (ffc-decomp ROADMAP #96, #38), admin only:
//   GET  /api/ffc/admin/bonus                     -> {"messages": [{id, text}]}
//   POST /api/ffc/admin/bonus {"secret","id","text"}         write/replace message id (1..64 one-shot, 65..96 news; ASCII, <= 1023 chars)
//   POST /api/ffc/admin/bonus {"secret","id","delete":true}  remove it
// Each id reaches a save once (the game flags it received), so a new announcement needs a new id. ffcbonus has the
// rules (ids 1..7 also give fossil rocks).

const ffcBonusGame = "VDEE"

type ffcBonusRequest struct {
	AuthInfo
	ID     int    `json:"id"`
	Text   string `json:"text"`
	Delete bool   `json:"delete"`
}

func HandleFFCBonus(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, err := parseGet(r, w, RoleModerator); err != nil {
			return
		}
		ms, err := ffcbonus.List(ffcBonusGame)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "bonus_error")
			return
		}
		replyOK(w, map[string]any{"messages": ms})
		return
	}
	var req ffcBonusRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	var err error
	if req.Delete {
		err = ffcbonus.Delete(ffcBonusGame, req.ID)
	} else {
		err = ffcbonus.Set(ffcBonusGame, req.ID, req.Text)
	}
	switch {
	case errors.Is(err, ffcbonus.ErrID), errors.Is(err, ffcbonus.ErrText):
		replyError(w, http.StatusBadRequest, APIErrorString(err.Error()))
		return
	case err != nil:
		replyError(w, http.StatusInternalServerError, "bonus_error")
		return
	}
	logging.Notice("API", "Bonus Data message", aurora.Cyan(req.ID), map[bool]string{true: "deleted", false: "set"}[req.Delete])
	replyOK(w, map[string]any{"id": req.ID, "deleted": req.Delete})
}
