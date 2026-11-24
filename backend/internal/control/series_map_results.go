package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) editSeriesMapResult(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MapNumber int    `json:"mapNumber"`
		Team1     *int   `json:"team1"`
		Team2     *int   `json:"team2"`
		Clear     bool   `json:"clear"`
		Revision  *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil || in.MapNumber < 1 || in.MapNumber > 5 || (in.Clear && (in.Team1 != nil || in.Team2 != nil)) || (!in.Clear && (in.Team1 == nil || in.Team2 == nil || *in.Team1 < 0 || *in.Team1 > 99 || *in.Team2 < 0 || *in.Team2 > 99)) {
		fail(w, 400, "Provide a map number and both scores from 0 to 99, or clear the result")
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
	err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF h`, r.PathValue("id")).Scan(&hostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var status, load string
	var revision int64
	var maps int
	err = tx.QueryRow(ctx, `SELECT m.status,m.load_status,m.result_revision,cardinality(m.series_maps) FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF m`, r.PathValue("id")).Scan(&status, &load, &revision, &maps)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != "closed" || load != "complete" || revision != *in.Revision || maps < 2 || in.MapNumber > maps {
		fail(w, 409, "The match, map or result changed; refresh and try again")
		return
	}
	if in.Clear {
		_, err = tx.Exec(ctx, `DELETE FROM match_series_map_results WHERE match_id=$1 AND map_number=$2`, r.PathValue("id"), in.MapNumber)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO match_series_map_results(match_id,map_number,team1_score,team2_score) VALUES($1,$2,$3,$4) ON CONFLICT(match_id,map_number) DO UPDATE SET team1_score=EXCLUDED.team1_score,team2_score=EXCLUDED.team2_score`, r.PathValue("id"), in.MapNumber, *in.Team1, *in.Team2)
	}
	if err != nil {
		internal(w, err)
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
