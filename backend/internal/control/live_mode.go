package control

import (
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *API) changeLiveMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode     string `json:"mode"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Mode != "match" && in.Mode != "practice" {
		fail(w, http.StatusBadRequest, "Choose Match or Practice mode")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	h, err := lockHost(ctx, tx, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if in.Revision == nil || *in.Revision != h.Revision {
		fail(w, http.StatusConflict, "Server state changed; refresh and try again")
		return
	}
	h.Project(time.Now())
	if !h.Actions.SwitchMode {
		fail(w, http.StatusConflict, "Live mode switching is unavailable; check MatchZy status and the current match phase")
		return
	}
	if in.Mode == h.ActivePreset {
		fail(w, http.StatusConflict, "Server is already in that mode")
		return
	}
	h.ModeCommandID = rand.Text()
	h.ModeCommandMode = in.Mode
	h.ModeCommandStatus = "pending"
	h.ModeCommandError = ""
	requested := time.Now()
	h.ModeCommandRequestedAt = &requested
	if _, err = tx.Exec(ctx, `UPDATE hosts SET mode_command_id=$2,mode_command_mode=$3,mode_command_status='pending',mode_command_error='',mode_command_requested_at=now(),revision=revision+1 WHERE id=$1`, h.ID, h.ModeCommandID, h.ModeCommandMode); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	h.Revision++
	h.Project(time.Now())
	respond(w, http.StatusAccepted, h)
}
