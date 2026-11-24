package control

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func advanceTournamentWinner(ctx context.Context, tx pgx.Tx, tournamentID, matchID string, round, number int, winner string) error {
	var format string
	if err := tx.QueryRow(ctx, `SELECT format FROM tournaments WHERE id=$1`, tournamentID).Scan(&format); err != nil {
		return err
	}
	if format == "double_elimination" {
		return advanceDoubleEliminationWinner(ctx, tx, tournamentID, matchID, winner)
	}
	if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET status='complete',winner_team_id=$2 WHERE id=$1 AND status='pending'`, matchID, winner); err != nil {
		return err
	}
	if format == "round_robin" {
		return finishRoundRobin(ctx, tx, tournamentID)
	}
	var tournamentWinner bool
	if err := tx.QueryRow(ctx, `SELECT $2=(SELECT max(round) FROM tournament_matches WHERE tournament_id=$1)`, tournamentID, round).Scan(&tournamentWinner); err != nil {
		return err
	}
	if tournamentWinner {
		if _, err := tx.Exec(ctx, `UPDATE tournaments SET status='complete',winner_team_id=$2 WHERE id=$1`, tournamentID, winner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1`, tournamentID, winner)
		return err
	}
	nextRound, nextNumber := round+1, (number+1)/2
	side := "team_a"
	if number%2 == 0 {
		side = "team_b"
	}
	if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET `+side+`=$3 WHERE tournament_id=$1 AND round=$2 AND number=$4`, tournamentID, nextRound, winner, nextNumber); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE tournament_matches SET veto_turn_team_id=team_a WHERE tournament_id=$1 AND round=$2 AND number=$3 AND team_a IS NOT NULL AND team_b IS NOT NULL AND veto_step=0`, tournamentID, nextRound, nextNumber)
	return err
}

func advanceDoubleEliminationWinner(ctx context.Context, tx pgx.Tx, tournamentID, matchID, winner string) error {
	var stage, teamA, teamB string
	var winnerNext, loserNext *string
	var winnerSlot, loserSlot *int
	if err := tx.QueryRow(ctx, `SELECT bracket_stage,team_a,team_b,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot FROM tournament_matches WHERE tournament_id=$1 AND id=$2 AND status='pending' FOR UPDATE`, tournamentID, matchID).Scan(&stage, &teamA, &teamB, &winnerNext, &winnerSlot, &loserNext, &loserSlot); err != nil {
		return err
	}
	if winner != teamA && winner != teamB {
		return errors.New("winner is not a participant")
	}
	loser := teamA
	if loser == winner {
		loser = teamB
	}
	if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET status='complete',winner_team_id=$2 WHERE id=$1 AND status='pending'`, matchID, winner); err != nil {
		return err
	}
	if stage == "grand_final" {
		if winner == teamA {
			if _, err := tx.Exec(ctx, `UPDATE tournaments SET status='complete',winner_team_id=$2 WHERE id=$1`, tournamentID, winner); err != nil {
				return err
			}
			if winnerNext != nil {
				if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET status='skipped' WHERE id=$1 AND bracket_stage='reset_final' AND status='pending'`, *winnerNext); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1`, tournamentID, winner)
			return err
		}
		if winnerNext == nil {
			return errors.New("grand final reset destination is missing")
		}
		_, err := tx.Exec(ctx, `UPDATE tournament_matches SET team_a=$2,team_b=$3,veto_turn_team_id=$2 WHERE id=$1 AND status='pending'`, *winnerNext, winner, loser)
		return err
	}
	if stage == "reset_final" {
		if _, err := tx.Exec(ctx, `UPDATE tournaments SET status='complete',winner_team_id=$2 WHERE id=$1`, tournamentID, winner); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=$2 THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1`, tournamentID, winner)
		return err
	}
	if winnerNext != nil && winnerSlot != nil {
		if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET team_a=CASE WHEN $2=1 THEN $3 ELSE team_a END,team_b=CASE WHEN $2=2 THEN $3 ELSE team_b END,veto_turn_team_id=CASE WHEN $2=1 THEN $3 ELSE COALESCE(veto_turn_team_id,team_a) END WHERE id=$1`, *winnerNext, *winnerSlot, winner); err != nil {
			return err
		}
	}
	var alreadyLost bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_matches WHERE tournament_id=$1 AND id<>$2 AND status='complete' AND winner_team_id<>$3 AND (team_a=$3 OR team_b=$3))`, tournamentID, matchID, loser).Scan(&alreadyLost); err != nil {
		return err
	}
	if alreadyLost {
		if _, err := tx.Exec(ctx, `UPDATE tournament_entries SET status='eliminated' WHERE tournament_id=$1 AND team_id=$2`, tournamentID, loser); err != nil {
			return err
		}
	}
	if loserNext != nil && loserSlot != nil {
		if _, err := tx.Exec(ctx, `UPDATE tournament_matches SET team_a=CASE WHEN $2=1 THEN $3 ELSE team_a END,team_b=CASE WHEN $2=2 THEN $3 ELSE team_b END WHERE id=$1`, *loserNext, *loserSlot, loser); err != nil {
			return err
		}
	}
	return settleDoubleEliminationByes(ctx, tx, tournamentID)
}

func reportedTournamentWinner(teamA, teamB, teamBSide string, scoreCT, scoreT int) (string, bool) {
	if teamA == "" || teamB == "" || (teamBSide != "ct" && teamBSide != "t") || scoreCT == scoreT {
		return "", false
	}
	winner := teamA
	teamBScore := scoreT
	if teamBSide == "ct" {
		teamBScore = scoreCT
	}
	if teamBScore > scoreCT+scoreT-teamBScore {
		winner = teamB
	}
	return winner, true
}

func reportedTournamentSeriesWinner(teamA, teamB, teamBSide string, bestOf int, score1, score2 *int) (string, bool) {
	if teamA == "" || teamB == "" || (teamBSide != "ct" && teamBSide != "t") || (bestOf != 3 && bestOf != 5) || score1 == nil || score2 == nil {
		return "", false
	}
	target := bestOf/2 + 1
	if *score1 > target || *score2 > target || *score1 == *score2 || (*score1 >= target) == (*score2 >= target) {
		return "", false
	}
	winner := teamA
	if teamBSide == "ct" {
		winner = teamB
	}
	if *score2 >= target {
		if winner == teamA {
			winner = teamB
		} else {
			winner = teamA
		}
	}
	return winner, true
}

// Auto-advance only from a durable, fenced MatchZy result. It runs after the
// machine heartbeat transaction commits so server-row locks cannot deadlock
// against tournament setup, which acquires tournament rows before host rows.
func (a *API) advanceTournamentFromReportedResult(ctx context.Context, hostID, setupID string) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tournamentID, matchID, teamA, teamB, teamBSide string
	var round, number, scoreCT, scoreT, bestOf int
	var score1, score2 *int
	err = tx.QueryRow(ctx, `SELECT tm.tournament_id,tm.id,tm.round,tm.number,tm.team_a,tm.team_b,tm.team_b_side,r.reported_ct,r.reported_t,t.best_of,r.reported_team1_score,r.reported_team2_score
		FROM match_rooms r JOIN tournament_matches tm ON tm.room_id=r.id JOIN tournaments t ON t.id=tm.tournament_id
		WHERE r.host_id=$1 AND r.load_command_id=$2 AND r.load_fence_required AND r.status='closed' AND r.load_status='complete'
		AND r.reported_ct IS NOT NULL AND r.reported_t IS NOT NULL AND tm.status='pending' AND t.status='active'
		AND tm.team_a IS NOT NULL AND tm.team_b IS NOT NULL AND tm.team_b_side IN ('ct','t')
		FOR UPDATE OF t,tm`, hostID, setupID).Scan(&tournamentID, &matchID, &round, &number, &teamA, &teamB, &teamBSide, &scoreCT, &scoreT, &bestOf, &score1, &score2)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var winner string
	var decided bool
	if bestOf > 1 {
		winner, decided = reportedTournamentSeriesWinner(teamA, teamB, teamBSide, bestOf, score1, score2)
	} else {
		winner, decided = reportedTournamentWinner(teamA, teamB, teamBSide, scoreCT, scoreT)
	}
	if !decided {
		return nil
	}
	if err = advanceTournamentWinner(ctx, tx, tournamentID, matchID, round, number, winner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
