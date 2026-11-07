package api

import (
	"errors"
	"net/http"
	"strconv"
	"wwfc/ffcbonus"
	"wwfc/ffcstats"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// FFC per-player mail inbox (ffc-decomp ROADMAP #77, s104-online), admin only:
//   GET  /api/ffc/admin/mail?secret=…&pid=P                -> {"pid": main, "mail": [{slot, text}]}
//   POST /api/ffc/admin/mail {"secret","pid","text"[,"slot"]}  write (slot 0/absent = first free; 1..16 replaces)
//   POST /api/ffc/admin/mail {"secret","pid","slot","delete":true}  remove one
//   POST /api/ffc/admin/mail {"secret","pid","clear":true}         empty the inbox
// pid may be any of the player's profiles: it is resolved to the main one (admin links, #53). The player sees the
// inbox on every Bonus Data visit while running the modded package (Bonus Data files 165..180, ffcbonus/mail.go).

type ffcMailRequest struct {
	AuthInfo
	Pid    uint32 `json:"pid"`
	Slot   int    `json:"slot"`
	Text   string `json:"text"`
	Delete bool   `json:"delete"`
	Clear  bool   `json:"clear"`
}

func HandleFFCMail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q, err := parseGet(r, w, RoleModerator)
		if err != nil {
			return
		}
		p, err := strconv.ParseUint(q.Get("pid"), 10, 32)
		if err != nil || p == 0 {
			replyError(w, http.StatusBadRequest, "bad_pid")
			return
		}
		pid := ffcstats.Resolve(ffcPlayerLinks(), uint32(p))
		ms, err := ffcbonus.MailList(ffcBonusGame, pid)
		if err != nil {
			replyError(w, http.StatusInternalServerError, "mail_error")
			return
		}
		replyOK(w, map[string]any{"pid": pid, "mail": ms})
		return
	}
	var req ffcMailRequest
	if err := parsePost(r, w, &req, RoleModerator); err != nil {
		return
	}
	if req.Pid == 0 {
		replyError(w, http.StatusBadRequest, "bad_pid")
		return
	}
	pid := ffcstats.Resolve(ffcPlayerLinks(), req.Pid)
	var err error
	slot := req.Slot
	what := "set"
	switch {
	case req.Clear:
		err, what = ffcbonus.MailClear(ffcBonusGame, pid), "cleared"
	case req.Delete:
		err, what = ffcbonus.MailDelete(ffcBonusGame, pid, slot), "deleted"
	default:
		slot, err = ffcbonus.MailSet(ffcBonusGame, pid, slot, req.Text)
	}
	switch {
	case errors.Is(err, ffcbonus.ErrSlot), errors.Is(err, ffcbonus.ErrText), errors.Is(err, ffcbonus.ErrFull):
		replyError(w, http.StatusBadRequest, APIErrorString(err.Error()))
		return
	case err != nil:
		replyError(w, http.StatusInternalServerError, "mail_error")
		return
	}
	logging.Notice("API", "FFC mail", aurora.Cyan(pid), "slot", aurora.Cyan(slot), what)
	replyOK(w, map[string]any{"pid": pid, "slot": slot, "result": what})
}
