package control

import (
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *API) updateReporter(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
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
	h.Project(time.Now())
	if !h.Actions.UpdateReporter {
		fail(w, 409, "A stopped server, connected agent and different staged reporter are required")
		return
	}
	h.ReporterCommandID = rand.Text()
	h.ReporterRequestedHash = h.ReporterSourceHash
	h.ReporterUpdateStatus = "pending"
	h.ReporterUpdateError = ""
	if _, err = tx.Exec(ctx, `UPDATE hosts SET reporter_command_id=$2,reporter_requested_hash=$3,reporter_command_status='pending',reporter_command_error='',reporter_command_requested_at=now(),revision=revision+1 WHERE id=$1`, h.ID, h.ReporterCommandID, h.ReporterRequestedHash); err != nil {
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
