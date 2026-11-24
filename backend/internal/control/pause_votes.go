package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) setPauseVoting(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Threshold int    `json:"threshold"`
		Revision  *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Threshold < 1 || in.Threshold > 10 {
		fail(w, 400, "Threshold must be between 1 and 10")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var hostID string
	err = tx.QueryRow(ctx, `SELECT host_id FROM match_rooms WHERE id=$1`, r.PathValue("id")).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	h, err := lockHost(ctx, tx, hostID)
	if err != nil {
		internal(w, err)
		return
	}
	if in.Revision == nil || *in.Revision != h.Revision {
		fail(w, 409, "Server state changed; refresh and try again")
		return
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&status); err != nil {
		internal(w, err)
		return
	}
	if status != "open" {
		fail(w, 409, "Match room is closed")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET pause_vote_threshold=$2 WHERE id=$1`, r.PathValue("id"), in.Threshold); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM match_pause_votes WHERE match_id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET revision=revision+1 WHERE id=$1`, hostID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]string{"status": "updated"})
}
