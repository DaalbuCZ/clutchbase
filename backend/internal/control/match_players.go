package control

import (
	"errors"
	"net/http"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

// Removing a room participant does not alter the server's access roster.
func (a *API) removeMatchPlayer(w http.ResponseWriter, r *http.Request) {
	steamID := r.PathValue("steam")
	if protocol.ValidateWhitelist([]string{steamID}) != nil {
		fail(w, http.StatusBadRequest, "Invalid player")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var editable bool
	err = tx.QueryRow(ctx, `SELECT m.status='open' AND m.load_status='' AND NOT EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=m.id) FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF m`, r.PathValue("id")).Scan(&editable)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !editable {
		fail(w, http.StatusConflict, "The room roster is locked after a load request or room closure")
		return
	}
	tag, err := tx.Exec(ctx, "DELETE FROM match_room_players WHERE match_id=$1 AND steam_id=$2", r.PathValue("id"), steamID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Player is no longer in the room")
		return
	}
	if _, err = tx.Exec(ctx, "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready=true", r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
