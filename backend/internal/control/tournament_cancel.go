package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) cancelTournament(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phase string `json:"phase"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Phase != "open" && in.Phase != "checkin" {
		fail(w, 400, "Choose an open registration or check-in to cancel")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var phase string
	err = tx.QueryRow(ctx, `SELECT status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&phase)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var fixtures bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_matches WHERE tournament_id=$1)`, r.PathValue("id")).Scan(&fixtures); err != nil {
		internal(w, err)
		return
	}
	if phase != in.Phase || fixtures {
		fail(w, 409, "The tournament changed or fixtures already started; wait for the latest snapshot")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='cancelled' WHERE id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
