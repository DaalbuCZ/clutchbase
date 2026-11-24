package control

import (
	"errors"
	"net/http"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

func (a *API) editMatchStats(w http.ResponseWriter, r *http.Request) {
	steam := r.PathValue("steam")
	if protocol.ValidateWhitelist([]string{steam}) != nil {
		fail(w, http.StatusBadRequest, "Invalid player")
		return
	}
	var in struct {
		Kills    *int   `json:"kills"`
		Deaths   *int   `json:"deaths"`
		Assists  *int   `json:"assists"`
		Clear    bool   `json:"clear"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	valid := func(n *int) bool { return n != nil && *n >= 0 && *n <= 999 }
	if in.Revision == nil || (in.Clear && (in.Kills != nil || in.Deaths != nil || in.Assists != nil)) || (!in.Clear && (!valid(in.Kills) || !valid(in.Deaths) || !valid(in.Assists))) {
		fail(w, http.StatusBadRequest, "Provide kills, deaths and assists from 0 to 999, or clear the stats")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var status, load string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT m.status,m.load_status,m.result_revision FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF m`, r.PathValue("id")).Scan(&status, &load, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != "closed" || load != "complete" || revision != *in.Revision {
		fail(w, http.StatusConflict, "The room or result changed; refresh and try again")
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE match_room_players SET kills=$3,deaths=$4,assists=$5 WHERE match_id=$1 AND steam_id=$2`, r.PathValue("id"), steam, in.Kills, in.Deaths, in.Assists)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(w, http.StatusNotFound, "Player is not in this match")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET result_revision=result_revision+1 WHERE id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
