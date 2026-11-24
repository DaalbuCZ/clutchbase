package control

import (
	"crypto/rand"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// requestServerRestart records stop intent; only a matching agent acknowledgement
// can authorize the separate start phase.
func (a *API) requestServerRestart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil {
		fail(w, 400, "Server revision is required")
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
	if h.Revision != *in.Revision {
		fail(w, 409, "Server state changed; refresh and try again")
		return
	}
	var capable, conflict bool
	err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT supports_restart AND driver='managed' AND last_seen>now()-interval '20 seconds' FROM machines WHERE id=$2),false),
 EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND (status='open' OR load_status='pending' OR cancel_status='pending')) OR
 EXISTS(SELECT 1 FROM game_updates WHERE host_id=$1 AND status='pending') OR
 EXISTS(SELECT 1 FROM server_restarts WHERE host_id=$1 AND phase IN ('stopping','starting','cancelling'))`, h.ID, h.MachineID).Scan(&capable, &conflict)
	if err != nil {
		internal(w, err)
		return
	}
	if !capable || !h.Online || h.Driver != "managed" || h.MachineID == "" || h.DesiredState != "running" || h.ObservedState != "running" || h.Error != "" || !h.WhitelistApplied || !h.PresetApplied || h.Removing || h.ReporterUpdateStatus == "pending" || h.ModeCommandStatus == "pending" || h.MatchControlStatus == "pending" || h.MatchPhase == "live" || h.MatchPhase == "paused" || h.MatchPhase == "setup" || (h.ActivePreset == "match" && h.MatchPhase != "idle") || conflict {
		fail(w, 409, "Restart is unavailable while the server, agent or match is busy")
		return
	}
	id := rand.Text()
	if _, err = tx.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES($1,$2,'stopping') ON CONFLICT(host_id) DO UPDATE SET request_id=EXCLUDED.request_id,phase='stopping',requested_at=now(),updated_at=now()`, h.ID, id); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET desired_state='stopped',revision=revision+1 WHERE id=$1`, h.ID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "stopping"})
}
