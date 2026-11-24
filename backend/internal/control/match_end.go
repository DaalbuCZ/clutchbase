package control

import (
	"crypto/rand"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

// The browser identifies a room; the command and its target come only from stored state.
func (a *API) endMatch(w http.ResponseWriter, r *http.Request) {
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
	var hostID string
	err = tx.QueryRow(ctx, "SELECT host_id FROM match_rooms WHERE id=$1", r.PathValue("id")).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	h, err := lockHost(ctx, tx, hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var setupID, activeID, loadStatus string
	var ended *time.Time
	err = tx.QueryRow(ctx, `SELECT m.load_command_id,m.load_status,m.ended_at,h.active_match_setup_id FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 FOR UPDATE OF m`, r.PathValue("id")).Scan(&setupID, &loadStatus, &ended, &activeID)
	if err != nil {
		internal(w, err)
		return
	}
	var loading, supported bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND load_status='pending')", hostID).Scan(&loading); err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT supports_match_control FROM machines WHERE id=$1),false)", h.MachineID).Scan(&supported); err != nil {
		internal(w, err)
		return
	}
	h.Project(time.Now())
	if !supported || loading || in.Revision == nil || *in.Revision != h.Revision || !h.Online || h.DesiredState != "running" || h.ObservedState != "running" || h.Driver != "managed" || h.ActivePreset != "match" || h.AccessStatus != "verified" || h.Error != "" || h.Removing || h.MatchControlStatus == "pending" || h.ModeCommandStatus == "pending" || (h.MatchPhase != "setup" && h.MatchPhase != "live" && h.MatchPhase != "paused") || loadStatus != "complete" || ended != nil || setupID == "" || setupID != activeID {
		fail(w, 409, "The loaded match or server state changed; refresh and try again")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE hosts SET match_control_id=$2,match_control_action='end',match_control_setup_id=$3,match_control_status='pending',match_control_error='',match_control_requested_at=now(),revision=revision+1 WHERE id=$1`, hostID, rand.Text(), setupID)
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "pending"})
}
