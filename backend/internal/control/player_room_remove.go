package control

import (
	"crypto/hmac"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) playerRemoveMatchPlayer(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		SettingsRevision  *int64 `json:"settingsRevision"`
		RosterFingerprint string `json:"rosterFingerprint"`
		RemovalToken      string `json:"removalToken"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.SettingsRevision == nil || len(in.RosterFingerprint) != 64 || len(in.RemovalToken) != 64 {
		fail(w, 400, "Review the current room roster before removing a player")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF h`, r.PathValue("id")).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Room unavailable for player management")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT id,settings_revision FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&id, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Room unavailable for player management")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	allowed, err := a.playerRoomPermission(ctx, tx, id, player.SteamID, false)
	if err != nil {
		internal(w, err)
		return
	}
	if !allowed {
		fail(w, 404, "Room unavailable for player management")
		return
	}
	if revision != *in.SettingsRevision {
		fail(w, 409, "The room changed; review it before removing a player")
		return
	}
	rows, err := tx.Query(ctx, `SELECT steam_id,side,ready FROM match_room_players WHERE match_id=$1 ORDER BY steam_id`, id)
	if err != nil {
		internal(w, err)
		return
	}
	var roster []shufflePlayer
	target := ""
	for rows.Next() {
		var p shufflePlayer
		if err = rows.Scan(&p.SteamID, &p.Side, &p.Ready); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		roster = append(roster, p)
		if p.SteamID != player.SteamID && hmac.Equal([]byte(a.participantRemovalToken(id, p.SteamID)), []byte(in.RemovalToken)) {
			target = p.SteamID
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if target == "" || !hmac.Equal([]byte(a.rosterFingerprint(id, roster)), []byte(in.RosterFingerprint)) {
		fail(w, 409, "The room roster changed; review it before removing a player")
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM match_room_players WHERE match_id=$1 AND steam_id=$2`, id, target); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready=true`, id); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
