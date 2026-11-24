package control

import (
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"clutchbase/internal/protocol"

	"github.com/jackc/pgx/v5"
)

func (a *API) changeMatchControl(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action   string `json:"action"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Action != "pause" && in.Action != "resume" {
		fail(w, http.StatusBadRequest, "Choose pause or resume")
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
	if (in.Action == "pause" && !h.Actions.PauseMatch) || (in.Action == "resume" && !h.Actions.ResumeMatch) {
		fail(w, http.StatusConflict, "Pause control requires a live MatchZy match and fresh runtime evidence")
		return
	}
	var setupID string
	if err = tx.QueryRow(ctx, "SELECT active_match_setup_id FROM hosts WHERE id=$1", h.ID).Scan(&setupID); err != nil {
		internal(w, err)
		return
	}
	if !protocol.ValidCommandID(setupID) {
		fail(w, http.StatusConflict, "The active match setup is unavailable")
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM match_pause_votes WHERE match_id IN (SELECT id FROM match_rooms WHERE host_id=$1)`, h.ID); err != nil {
		internal(w, err)
		return
	}
	h.MatchControlID = rand.Text()
	h.MatchControlAction = in.Action
	h.MatchControlStatus = "pending"
	h.MatchControlError = ""
	requested := time.Now()
	h.MatchControlRequestedAt = &requested
	if _, err = tx.Exec(ctx, `UPDATE hosts SET match_control_id=$2,match_control_action=$3,match_control_setup_id=$4,match_control_status='pending',match_control_error='',match_control_requested_at=now(),revision=revision+1 WHERE id=$1`, h.ID, h.MatchControlID, in.Action, setupID); err != nil {
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
