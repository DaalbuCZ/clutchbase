package control

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *API) removeServer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var machineID string
	err = tx.QueryRow(r.Context(), "SELECT COALESCE(machine_id,'') FROM hosts WHERE id=$1 AND removed_at IS NULL", r.PathValue("id")).Scan(&machineID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if machineID == "" {
		fail(w, 409, "Only machine-managed servers support removal")
		return
	}
	// Use the same lock order as fleet reports and port allocation.
	if _, err = tx.Exec(r.Context(), "SELECT id FROM machines WHERE id=$1 FOR UPDATE", machineID); err != nil {
		internal(w, err)
		return
	}
	h, err := lockHost(r.Context(), tx, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if in.Revision == nil || *in.Revision != h.Revision {
		fail(w, 409, "Server state changed; refresh and try again")
		return
	}
	if !h.Actions.Remove {
		fail(w, 409, "Removal requires a stopped server and an online agent that supports removal")
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE hosts SET remove_requested=true,revision=revision+1 WHERE id=$1", h.ID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	h.Removing = true
	h.Revision++
	h.Project(time.Now())
	respond(w, 202, h)
}
