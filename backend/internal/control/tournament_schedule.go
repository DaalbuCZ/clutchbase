package control

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *API) scheduleTournamentMatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision    *int64          `json:"revision"`
		ScheduledAt json.RawMessage `json:"scheduledAt"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision == nil || *in.Revision < 0 || len(in.ScheduledAt) == 0 {
		fail(w, http.StatusBadRequest, "revision and scheduledAt are required")
		return
	}
	var at *time.Time
	if string(in.ScheduledAt) != "null" {
		var value string
		if json.Unmarshal(in.ScheduledAt, &value) != nil {
			fail(w, http.StatusBadRequest, "scheduledAt must be null or RFC3339")
			return
		}
		v, err := time.Parse(time.RFC3339, value)
		if err != nil || v.Year() < 1970 || v.Year() > 2100 {
			fail(w, http.StatusBadRequest, "scheduledAt must be a valid date between 1970 and 2100")
			return
		}
		at = &v
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var tournamentStatus, matchStatus string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&tournamentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Tournament fixture not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	err = tx.QueryRow(ctx, `SELECT status,schedule_revision FROM tournament_matches WHERE tournament_id=$1 AND id=$2 FOR UPDATE`, r.PathValue("id"), r.PathValue("match")).Scan(&matchStatus, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Tournament fixture not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if revision != *in.Revision {
		fail(w, http.StatusConflict, "Schedule revision is stale")
		return
	}
	if tournamentStatus != "active" || matchStatus != "pending" {
		fail(w, http.StatusConflict, "Only active pending fixtures can be scheduled")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET scheduled_at=$3,schedule_revision=schedule_revision+1 WHERE tournament_id=$1 AND id=$2`, r.PathValue("id"), r.PathValue("match"), at); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
