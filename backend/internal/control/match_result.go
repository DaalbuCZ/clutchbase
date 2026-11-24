package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) editMatchResult(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CT       *int   `json:"ct"`
		T        *int   `json:"t"`
		Clear    bool   `json:"clear"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil || (in.Clear && (in.CT != nil || in.T != nil)) || (!in.Clear && (in.CT == nil || in.T == nil || *in.CT < 0 || *in.CT > 99 || *in.T < 0 || *in.T > 99)) {
		fail(w, http.StatusBadRequest, "Provide both scores from 0 to 99, or clear the result")
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
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET result_ct=$2,result_t=$3,result_revision=result_revision+1 WHERE id=$1`, r.PathValue("id"), in.CT, in.T); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
