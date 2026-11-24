package control

import (
	"errors"
	"net/http"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

// The snapshot includes only anonymous counts; each player receives their own vote separately.
const matchVotesSQL = `COALESCE((SELECT json_agg(json_build_object('map',v.map,'votes',v.votes) ORDER BY v.votes DESC,v.map) FROM (SELECT map,count(*)::int votes FROM match_map_votes WHERE match_id=m.id AND steam_id=ANY(h.whitelist) GROUP BY map) v),'[]'::json)`

func (a *API) voteMatchMap(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Map string `json:"map"`
	}
	if r.Method == http.MethodPut {
		if !decode(w, r, &in) {
			return
		}
		if !protocol.ValidMap(in.Map) {
			fail(w, 400, "Choose a supported official map")
			return
		}
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var host string
	err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL AND $2=ANY(h.whitelist) FOR UPDATE OF h`, r.PathValue("id"), player.SteamID).Scan(&host)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var room string
	if err = tx.QueryRow(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&room); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Match room not found")
			return
		}
		internal(w, err)
		return
	}
	var status, load string
	var tournament, joined bool
	err = tx.QueryRow(ctx, `SELECT m.status,m.load_status,EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=m.id),EXISTS(SELECT 1 FROM match_room_players WHERE match_id=m.id AND steam_id=$2) FROM match_rooms m WHERE m.id=$1`, r.PathValue("id"), player.SteamID).Scan(&status, &load, &tournament, &joined)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !joined || tournament || status != "open" || load != "" {
		fail(w, 409, "Voting is available to participants in open standalone rooms before loading")
		return
	}
	if r.Method == http.MethodPut {
		_, err = tx.Exec(ctx, `INSERT INTO match_map_votes(match_id,steam_id,map) VALUES($1,$2,$3) ON CONFLICT(match_id,steam_id) DO UPDATE SET map=excluded.map WHERE match_map_votes.map<>excluded.map`, r.PathValue("id"), player.SteamID, in.Map)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM match_map_votes WHERE match_id=$1 AND steam_id=$2`, r.PathValue("id"), player.SteamID)
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
