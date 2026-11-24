package control

import (
	"crypto/rand"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Use the same predicate for the projected action and the locked mutation.
const canCancelMatchSQL = `m.load_status='failed' AND m.load_expiry_enforced AND m.load_fence_required
	AND m.load_command_id ~ '^[A-Z2-7]{26}$' AND COALESCE(m.load_payload->>'id'=m.load_command_id,false)
	AND COALESCE(m.load_payload->>'fenceRequired'='true',false) AND m.cancel_status<>'pending'
	AND h.desired_state='running' AND h.observed_state='running' AND h.last_seen>now()-interval '20 seconds'
	AND h.driver='managed' AND h.access_status='verified' AND h.active_preset='match'
	AND h.match_phase='idle' AND h.can_load_match AND h.active_match_setup_id='' AND h.supports_match_fence AND h.error='' AND NOT h.remove_requested
	AND h.match_control_status<>'pending' AND h.mode_command_status<>'pending'
	AND COALESCE((SELECT supports_match_cancel FROM machines WHERE id=h.machine_id),false)
	AND NOT EXISTS (SELECT 1 FROM match_rooms pending WHERE pending.host_id=h.id AND (pending.load_status='pending' OR pending.cancel_status='pending'))`

func (a *API) cancelMatchLoad(w http.ResponseWriter, r *http.Request) {
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
	var revision int64
	err = tx.QueryRow(ctx, "SELECT revision FROM hosts WHERE id=$1 AND removed_at IS NULL FOR UPDATE", hostID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT `+canCancelMatchSQL+` FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 FOR UPDATE OF m`, r.PathValue("id")).Scan(&eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if in.Revision == nil || *in.Revision != revision || !eligible {
		fail(w, 409, "The failed load cannot be cancelled with the current server evidence")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET cancel_id=$2,cancel_status='pending',cancel_error='' WHERE id=$1`, r.PathValue("id"), rand.Text()); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "pending"})
}
