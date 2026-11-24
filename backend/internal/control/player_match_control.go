package control

import (
	"crypto/rand"
	"errors"
	"net/http"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

func (a *API) playerMatchControl(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Action   string `json:"action"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Action != "pause" && in.Action != "resume" {
		fail(w, 400, "Choose pause or resume")
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
	err = tx.QueryRow(ctx, `SELECT m.host_id FROM match_rooms m JOIN hosts h ON h.id=m.host_id JOIN match_room_players p ON p.match_id=m.id WHERE m.id=$1 AND p.steam_id=$2 AND $2=ANY(h.whitelist) AND h.removed_at IS NULL`, r.PathValue("id"), player.SteamID).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Joined match room not found")
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
	var setupID, activeID, status, loadStatus string
	var ended *time.Time
	var permitted bool
	err = tx.QueryRow(ctx, `SELECT m.load_command_id,h.active_match_setup_id,m.status,m.load_status,m.ended_at,($2=ANY(h.whitelist) AND EXISTS(SELECT 1 FROM match_room_players WHERE match_id=m.id AND steam_id=$2)) FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 FOR UPDATE OF m`, r.PathValue("id"), player.SteamID).Scan(&setupID, &activeID, &status, &loadStatus, &ended, &permitted)
	if err != nil {
		internal(w, err)
		return
	}
	h.Project(time.Now())
	if !permitted || in.Revision == nil || *in.Revision != h.Revision || status != "open" || loadStatus != "complete" || ended != nil || !protocol.ValidCommandID(setupID) || setupID != activeID || h.AccessStatus != "verified" || h.ActivePreset != "match" || (in.Action == "pause" && !h.Actions.PauseMatch) || (in.Action == "resume" && !h.Actions.ResumeMatch) {
		fail(w, 409, "The loaded match or server state changed; wait for the latest match snapshot")
		return
	}
	if in.Action == "pause" {
		var threshold, votes int
		if err = tx.QueryRow(ctx, `SELECT pause_vote_threshold FROM match_rooms WHERE id=$1`, r.PathValue("id")).Scan(&threshold); err != nil {
			internal(w, err)
			return
		}
		tag, err := tx.Exec(ctx, `INSERT INTO match_pause_votes(match_id,steam_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, r.PathValue("id"), player.SteamID)
		if err != nil {
			internal(w, err)
			return
		}
		if tag.RowsAffected() == 0 {
			fail(w, 409, "You have already voted to pause")
			return
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM match_pause_votes WHERE match_id=$1`, r.PathValue("id")).Scan(&votes); err != nil {
			internal(w, err)
			return
		}
		if votes < threshold {
			if _, err = tx.Exec(ctx, `UPDATE hosts SET revision=revision+1 WHERE id=$1`, hostID); err != nil {
				internal(w, err)
				return
			}
			if err = tx.Commit(ctx); err != nil {
				internal(w, err)
				return
			}
			respond(w, http.StatusAccepted, map[string]string{"status": "voted"})
			return
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM match_pause_votes WHERE match_id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET match_control_id=$2,match_control_action=$3,match_control_setup_id=$4,match_control_status='pending',match_control_error='',match_control_requested_at=now(),revision=revision+1 WHERE id=$1`, hostID, rand.Text(), in.Action, setupID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "pending"})
}
