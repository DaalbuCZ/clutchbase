package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) withdrawTournamentTeam(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		TeamRevision *int64 `json:"teamRevision"`
		TeamID       string `json:"teamId"`
		EntryStatus  string `json:"entryStatus"`
		Phase        string `json:"phase"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TeamRevision == nil || (in.Phase != "open" && in.Phase != "checkin") || in.TeamID == "" || (in.EntryStatus != "registered" && in.EntryStatus != "checked_in") {
		fail(w, 400, "Choose the current tournament registration")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	// Registration, check-in and fixture generation take the tournament lock first.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != in.Phase || (status != "open" && status != "checkin") {
		fail(w, 409, "Registration cannot be withdrawn after fixtures start")
		return
	}
	var team string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT id,revision FROM teams WHERE id=$1 AND captain_steam_id=$2 AND disbanded_at IS NULL FOR UPDATE`, in.TeamID, player.SteamID).Scan(&team, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Team not found or captain access required")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if revision != *in.TeamRevision {
		fail(w, 409, "The team changed; wait for the latest tournament snapshot")
		return
	}
	var fixtures bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_matches WHERE tournament_id=$1)`, r.PathValue("id")).Scan(&fixtures); err != nil {
		internal(w, err)
		return
	}
	if fixtures {
		fail(w, 409, "Fixtures already exist; registration is locked")
		return
	}
	result, err := tx.Exec(ctx, `DELETE FROM tournament_entries WHERE tournament_id=$1 AND team_id=$2 AND status=$3`, r.PathValue("id"), team, in.EntryStatus)
	if err != nil {
		internal(w, err)
		return
	}
	if result.RowsAffected() != 1 {
		fail(w, 409, "The registration changed; wait for the latest tournament snapshot")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE teams SET revision=revision+1 WHERE id=$1`, team); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
