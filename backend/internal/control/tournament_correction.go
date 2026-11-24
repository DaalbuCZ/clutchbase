package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// correctTournamentWinner preserves reported scores while correcting outcomes.
func (a *API) correctTournamentWinner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TeamID   string `json:"teamId"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.TeamID == "" || in.Revision == nil || *in.Revision < 0 {
		fail(w, 400, "Team and result revision are required")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var format, status string
	err = tx.QueryRow(ctx, `SELECT format,status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&format, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != "active" && status != "complete" {
		fail(w, 409, "Correction is not available for this tournament")
		return
	}
	var teamA, teamB, winner string
	var revision int64
	var terminal bool
	var round, number int
	var stage string
	var winnerNext, loserNext *string
	var winnerSlot, loserSlot *int
	err = tx.QueryRow(ctx, `SELECT team_a,team_b,winner_team_id,result_revision,((bracket_stage='single' AND NOT EXISTS(SELECT 1 FROM tournament_matches later WHERE later.tournament_id=m.tournament_id AND later.round>m.round)) OR bracket_stage='reset_final'),round,number,bracket_stage,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot FROM tournament_matches m WHERE tournament_id=$1 AND id=$2 AND status='complete' FOR UPDATE`, r.PathValue("id"), r.PathValue("match")).Scan(&teamA, &teamB, &winner, &revision, &terminal, &round, &number, &stage, &winnerNext, &winnerSlot, &loserNext, &loserSlot)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Completed fixture not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	grandReset := format == "double_elimination" && stage == "grand_final" && status == "complete" && winner == teamA && winnerNext != nil
	grandFinish := format == "double_elimination" && stage == "grand_final" && status == "active" && winner == teamB && winnerNext != nil
	if format == "double_elimination" && !terminal && !grandReset && !grandFinish && (status != "active" || (stage != "winners" && stage != "losers") || winnerNext == nil || winnerSlot == nil || (stage == "winners" && (loserNext == nil || loserSlot == nil))) {
		fail(w, 409, "Later fixtures depend on this result; correction is unavailable")
		return
	}
	if revision != *in.Revision {
		fail(w, 409, "Fixture changed; review the current result")
		return
	}
	if in.TeamID != teamA && in.TeamID != teamB {
		fail(w, 400, "Winner must be one of the fixture teams")
		return
	}
	if in.TeamID == winner {
		fail(w, 409, "This team is already the winner")
		return
	}
	if grandReset || grandFinish {
		var resetStatus string
		var resetA, resetB, resetWinner, room *string
		var veto int
		err = tx.QueryRow(ctx, `SELECT status,team_a,team_b,winner_team_id,room_id,veto_step FROM tournament_matches WHERE id=$1 AND tournament_id=$2 AND bracket_stage='reset_final' FOR UPDATE`, *winnerNext, r.PathValue("id")).Scan(&resetStatus, &resetA, &resetB, &resetWinner, &room, &veto)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 409, "Reset fixture is unavailable")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		untouchedSkipped := grandReset && resetStatus == "skipped" && resetA == nil && resetB == nil
		untouchedPending := grandFinish && resetStatus == "pending" && resetA != nil && *resetA == teamB && resetB != nil && *resetB == teamA
		if (!untouchedSkipped && !untouchedPending) || resetWinner != nil || room != nil || veto != 0 {
			fail(w, 409, "Reset fixture has changed; correction is unavailable")
			return
		}
		if grandFinish {
			if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET status='skipped',team_a=NULL,team_b=NULL,veto_turn_team_id=NULL WHERE id=$1`, *winnerNext); err != nil {
				internal(w, err)
				return
			}
			if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='complete',winner_team_id=$2 WHERE id=$1`, r.PathValue("id"), teamA); err != nil {
				internal(w, err)
				return
			}
			if _, err = tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1`, r.PathValue("id"), teamA); err != nil {
				internal(w, err)
				return
			}
		} else if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET status='pending',team_a=$2,team_b=$3,veto_turn_team_id=$2 WHERE id=$1`, *winnerNext, teamB, teamA); err != nil {
			internal(w, err)
			return
		}
		if grandReset {
			if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='active',winner_team_id=NULL WHERE id=$1`, r.PathValue("id")); err != nil {
				internal(w, err)
				return
			}
			if _, err = tx.Exec(ctx, `UPDATE tournament_entries SET status='checked_in' WHERE tournament_id=$1 AND team_id IN ($2,$3)`, r.PathValue("id"), teamA, teamB); err != nil {
				internal(w, err)
				return
			}
		}
	}
	if format == "double_elimination" && !terminal && !grandReset && !grandFinish {
		oldLoser := teamA
		if oldLoser == winner {
			oldLoser = teamB
		}
		routes := []struct {
			id               string
			slot             int
			old, replacement string
		}{{*winnerNext, *winnerSlot, winner, in.TeamID}}
		if stage == "winners" {
			routes = append(routes, struct {
				id               string
				slot             int
				old, replacement string
			}{*loserNext, *loserSlot, oldLoser, winner})
		}
		if stage == "losers" {
			var winnerStatus, loserStatus string
			if err = tx.QueryRow(ctx, `SELECT (SELECT status FROM tournament_entries WHERE tournament_id=$1 AND team_id=$2),(SELECT status FROM tournament_entries WHERE tournament_id=$1 AND team_id=$3)`, r.PathValue("id"), winner, oldLoser).Scan(&winnerStatus, &loserStatus); err != nil {
				internal(w, err)
				return
			}
			if winnerStatus != "checked_in" || loserStatus != "eliminated" {
				fail(w, 409, "Team standings have changed; correction is unavailable")
				return
			}
		}
		for _, route := range routes {
			var nextStatus string
			var slot *string
			var veto int
			var room *string
			err = tx.QueryRow(ctx, `SELECT status,CASE WHEN $2=1 THEN team_a ELSE team_b END,veto_step,room_id FROM tournament_matches WHERE id=$1 AND tournament_id=$3 FOR UPDATE`, route.id, route.slot, r.PathValue("id")).Scan(&nextStatus, &slot, &veto, &room)
			if errors.Is(err, pgx.ErrNoRows) {
				fail(w, 409, "Next fixture is unavailable")
				return
			}
			if err != nil {
				internal(w, err)
				return
			}
			if nextStatus != "pending" || veto != 0 || room != nil || slot == nil || *slot != route.old {
				fail(w, 409, "Next fixture has changed or started; correction is unavailable")
				return
			}
		}
		for _, route := range routes {
			if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET team_a=CASE WHEN $2=1 THEN $3 ELSE team_a END,team_b=CASE WHEN $2=2 THEN $3 ELSE team_b END,veto_turn_team_id=CASE WHEN $2=1 AND team_b IS NOT NULL THEN $3 WHEN $2=2 AND team_a IS NOT NULL THEN team_a ELSE NULL END WHERE id=$1`, route.id, route.slot, route.replacement); err != nil {
				internal(w, err)
				return
			}
		}
		if stage == "losers" {
			if _, err = tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'eliminated' ELSE 'checked_in' END WHERE tournament_id=$1 AND team_id IN ($2,$3)`, r.PathValue("id"), winner, oldLoser); err != nil {
				internal(w, err)
				return
			}
		}
	}
	if format == "single_elimination" && !terminal {
		if status != "active" {
			fail(w, 409, "Later fixtures depend on this result; correction is unavailable")
			return
		}
		var nextID, nextStatus string
		var slot *string
		var veto int
		var room *string
		err = tx.QueryRow(ctx, `SELECT id,status,CASE WHEN $3%2=1 THEN team_a ELSE team_b END,veto_step,room_id FROM tournament_matches WHERE tournament_id=$1 AND round=$2+1 AND number=($3+1)/2 FOR UPDATE`, r.PathValue("id"), round, number).Scan(&nextID, &nextStatus, &slot, &veto, &room)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 409, "Next fixture is unavailable")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		if nextStatus != "pending" || veto != 0 || room != nil || slot == nil || *slot != winner {
			fail(w, 409, "Next fixture has changed or started; correction is unavailable")
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET team_a=CASE WHEN $2%2=1 THEN $3 ELSE team_a END,team_b=CASE WHEN $2%2=0 THEN $3 ELSE team_b END,veto_turn_team_id=CASE WHEN $2%2=1 AND team_b IS NOT NULL THEN $3 WHEN $2%2=0 AND team_a IS NOT NULL THEN team_a ELSE NULL END WHERE id=$1`, nextID, number, in.TeamID); err != nil {
			internal(w, err)
			return
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET winner_team_id=$2 WHERE id=$1`, r.PathValue("match"), in.TeamID); err != nil {
		internal(w, err)
		return
	}
	if status == "complete" && format == "round_robin" {
		if _, err = tx.Exec(ctx, `UPDATE tournament_entries SET status='checked_in' WHERE tournament_id=$1 AND status IN ('winner','eliminated')`, r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
		if err = finishRoundRobin(ctx, tx, r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
	} else if status == "complete" && !grandReset {
		if _, err = tx.Exec(ctx, `UPDATE tournaments SET winner_team_id=$2 WHERE id=$1`, r.PathValue("id"), in.TeamID); err != nil {
			internal(w, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1 AND status IN ('winner','eliminated')`, r.PathValue("id"), in.TeamID); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
