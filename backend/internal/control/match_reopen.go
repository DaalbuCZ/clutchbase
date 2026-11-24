package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) reopenMatch(w http.ResponseWriter, r *http.Request) {
	a.reopenMatchFor(w, r, "")
}

func (a *API) playerReopenMatch(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if ok { a.reopenMatchFor(w, r, player.SteamID) }
}

func (a *API) reopenMatchFor(w http.ResponseWriter, r *http.Request, steam string) {
	var in struct { SettingsRevision *int64 `json:"settingsRevision"` }
	if !decode(w, r, &in) { return }
	if in.SettingsRevision == nil { fail(w, 400, "Room settings revision required"); return }
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil { internal(w, err); return }
	defer tx.Rollback(ctx)
	var hostID string
	if err = tx.QueryRow(ctx, `SELECT host_id FROM match_rooms WHERE id=$1`, r.PathValue("id")).Scan(&hostID); errors.Is(err, pgx.ErrNoRows) { fail(w, 404, "Match room not found"); return } else if err != nil { internal(w, err); return }
	h, err := lockHost(ctx, tx, hostID)
	if errors.Is(err, pgx.ErrNoRows) { fail(w, 404, "Server not found"); return } else if err != nil { internal(w, err); return }
	var status, load, mapName, owner string
	var revision int64
	var tournament, joined, busy bool
	err = tx.QueryRow(ctx, `SELECT status,load_status,map,settings_revision,COALESCE(created_by_steam_id,''),EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=m.id),EXISTS(SELECT 1 FROM match_room_players WHERE match_id=m.id AND steam_id=$2) FROM match_rooms m WHERE id=$1 FOR UPDATE`, r.PathValue("id"), steam).Scan(&status, &load, &mapName, &revision, &owner, &tournament, &joined)
	if errors.Is(err, pgx.ErrNoRows) { fail(w, 404, "Match room not found"); return } else if err != nil { internal(w, err); return }
	if steam != "" {
		allowed := false
		for _, id := range h.Whitelist { if id == steam { allowed = true; break } }
		if owner != steam || !joined || !allowed || !h.AllowPlayerRooms || playerJoinBlocker(h) != "" { fail(w, 404, "Room unavailable for player management"); return }
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND id<>$2 AND (status='open' OR load_status='pending' OR cancel_status='pending'))`, hostID, r.PathValue("id")).Scan(&busy); err != nil { internal(w, err); return }
	if status != "closed" || load != "" || tournament || revision != *in.SettingsRevision || busy || !canOpenMatch(h) || !hostHasMap(h, mapName) {
		fail(w, 409, "Room or server changed; reopen is unavailable")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready`, r.PathValue("id")); err != nil { internal(w, err); return }
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET status='open',closed_at=NULL,settings_revision=settings_revision+1 WHERE id=$1`, r.PathValue("id")); err != nil { internal(w, err); return }
	if err = tx.Commit(ctx); err != nil { internal(w, err); return }
	w.WriteHeader(http.StatusNoContent)
}
