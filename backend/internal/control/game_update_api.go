package control

import (
	"crypto/rand"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) updateGame(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision  *int64 `json:"revision"`
		StopFirst bool   `json:"stopFirst"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil {
		fail(w, http.StatusBadRequest, "Server revision is required")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	h, err := lockHost(r.Context(), tx, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if h.Revision != *in.Revision {
		fail(w, http.StatusConflict, "Server state changed; refresh and try again")
		return
	}
	if in.StopFirst {
		var matchBusy bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND (status='open' OR load_status='pending' OR cancel_status='pending'))`, h.ID).Scan(&matchBusy); err != nil {
			internal(w, err)
			return
		}
		if !h.Actions.StopAndUpdate || matchBusy {
			fail(w, http.StatusConflict, "Stop and update is unavailable while the server, agent or match is busy")
			return
		}
	} else if !h.Actions.UpdateGame {
		fail(w, http.StatusConflict, "Connect an updated managed agent and stop the server before updating CS2")
		return
	}
	id := rand.Text()
	if _, err = tx.Exec(r.Context(), `INSERT INTO game_updates(host_id,request_id,status) VALUES($1,$2,'pending') ON CONFLICT(host_id) DO UPDATE SET request_id=EXCLUDED.request_id,status='pending',error='',issued_at=NULL,requested_at=now(),updated_at=now()`, h.ID, id); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE hosts SET desired_state=CASE WHEN $2 THEN 'stopped' ELSE desired_state END,revision=revision+1 WHERE id=$1", h.ID, in.StopFirst); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

// Cancellation is allowed only until an update command is committed for delivery.
func (a *API) cancelGameUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil {
		fail(w, http.StatusBadRequest, "Server revision is required")
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
	if h.Revision != *in.Revision || !h.Actions.CancelGameUpdate {
		fail(w, http.StatusConflict, "The update has changed or has already been sent to the agent")
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE game_updates SET status='cancelled',updated_at=now() WHERE host_id=$1 AND status='pending' AND issued_at IS NULL`, h.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, http.StatusConflict, "The update has already been sent to the agent")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET revision=revision+1 WHERE id=$1`, h.ID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
