package control

import (
	"context"
	"crypto/rand"
	"math/bits"

	"github.com/jackc/pgx/v5"
)

type doubleBracketMatch struct {
	id, stage               string
	round, number           int
	a, b                    string
	winnerMatch, loserMatch string
	winnerSlot, loserSlot   int
	sourceA, sourceB        string
}

// doubleEliminationBracket builds a persisted routing graph. Team IDs are
// sorted by the caller so the same registrations always produce the same draw.
func doubleEliminationBracket(teams []string) []doubleBracketMatch {
	if len(teams) < 2 {
		return nil
	}
	size := 1 << bits.Len(uint(len(teams)-1))
	winners := make([][]int, bits.Len(uint(size)))
	loserRounds := 2*(bits.Len(uint(size))-1) - 2
	if loserRounds < 0 {
		loserRounds = 0
	}
	losers := make([][]int, loserRounds+1)
	matches := []doubleBracketMatch{}
	add := func(stage string, round, count int) []int {
		ids := make([]int, count)
		for n := range ids {
			ids[n] = len(matches)
			matches = append(matches, doubleBracketMatch{id: rand.Text(), stage: stage, round: round, number: n + 1})
		}
		return ids
	}
	wRounds := bits.Len(uint(size)) - 1
	for r := 1; r <= wRounds; r++ {
		winners[r] = add("winners", r, size>>r)
	}
	if wRounds > 1 {
		for r := 1; r <= loserRounds; r++ {
			count := size >> ((r+1)/2 + 1)
			losers[r] = add("losers", wRounds+r, count)
		}
	}
	grand := add("grand_final", wRounds+loserRounds+1, 1)[0]
	reset := add("reset_final", wRounds+loserRounds+2, 1)[0]
	for i, team := range teams {
		if i%2 == 0 {
			matches[winners[1][i/2]].a = team
		} else {
			matches[winners[1][i/2]].b = team
		}
	}
	setRoute := func(from, to int, slot int, loser bool) {
		if loser {
			matches[from].loserMatch = matches[to].id
			matches[from].loserSlot = slot
		} else {
			matches[from].winnerMatch = matches[to].id
			matches[from].winnerSlot = slot
		}
		if slot == 1 {
			matches[to].sourceA = matches[from].id
		} else {
			matches[to].sourceB = matches[from].id
		}
	}
	for r := 1; r < wRounds; r++ {
		for i, from := range winners[r] {
			to := winners[r+1][i/2]
			slot := i%2 + 1
			setRoute(from, to, slot, false)
			if r == 1 && wRounds > 1 {
				setRoute(from, losers[1][i/2], i%2+1, true)
			}
			if r >= 2 && wRounds > 1 {
				setRoute(from, losers[2*r-2][i], 2, true)
			}
		}
	}
	if wRounds > 1 {
		for r := 1; r < loserRounds; r++ {
			current, next := losers[r], losers[r+1]
			if len(next) == len(current) {
				for i, from := range current {
					setRoute(from, next[i], 1, false)
				}
			} else {
				for i, from := range current {
					setRoute(from, next[i/2], i%2+1, false)
				}
			}
		}
	}
	for _, from := range winners[wRounds] {
		matches[from].winnerMatch = matches[grand].id
		matches[from].winnerSlot = 1
		matches[grand].sourceA = matches[from].id
		if wRounds == 1 {
			matches[from].loserMatch = matches[grand].id
			matches[from].loserSlot = 2
			matches[grand].sourceB = matches[from].id
		} else {
			lastLosers := losers[loserRounds][0]
			matches[from].loserMatch = matches[lastLosers].id
			matches[from].loserSlot = 2
			matches[lastLosers].sourceB = matches[from].id
		}
	}
	if wRounds > 1 {
		lastLosers := losers[loserRounds][0]
		matches[lastLosers].winnerMatch = matches[grand].id
		matches[lastLosers].winnerSlot = 2
		matches[grand].sourceB = matches[lastLosers].id
	}
	matches[grand].winnerMatch = matches[reset].id
	matches[grand].winnerSlot = 1
	matches[reset].sourceA = matches[grand].id
	matches[reset].sourceB = matches[grand].id
	return matches
}

func settleDoubleEliminationByes(ctx context.Context, tx pgx.Tx, tournamentID string) error {
	for {
		rows, err := tx.Query(ctx, `SELECT id,team_a,team_b,source_a_match_id,source_b_match_id FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage IN ('winners','losers') AND status='pending' ORDER BY round,number`, tournamentID)
		if err != nil {
			return err
		}
		type candidate struct {
			id                     string
			a, b, sourceA, sourceB *string
		}
		matches := []candidate{}
		for rows.Next() {
			var m candidate
			if err = rows.Scan(&m.id, &m.a, &m.b, &m.sourceA, &m.sourceB); err != nil {
				rows.Close()
				return err
			}
			matches = append(matches, m)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		changed := false
		for _, m := range matches {
			readyA, readyB := true, true
			if m.sourceA != nil {
				var ok bool
				if m.a, ok, err = doubleEliminationSource(ctx, tx, *m.sourceA, m.id, 1); err != nil {
					return err
				}
				readyA = ok
			}
			if m.sourceB != nil {
				var ok bool
				if m.b, ok, err = doubleEliminationSource(ctx, tx, *m.sourceB, m.id, 2); err != nil {
					return err
				}
				readyB = ok
			}
			if !readyA || !readyB {
				continue
			}
			if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET team_a=$2::text,team_b=$3::text,veto_turn_team_id=CASE WHEN $2::text IS NOT NULL AND $3::text IS NOT NULL AND veto_step=0 THEN $2::text ELSE veto_turn_team_id END WHERE id=$1 AND status='pending'`, m.id, m.a, m.b); err != nil {
				return err
			}
			if (m.a == nil) != (m.b == nil) || (m.a == nil && m.b == nil) {
				var winner any
				if m.a != nil {
					winner = *m.a
				}
				if m.b != nil {
					winner = *m.b
				}
				if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET status='bye',winner_team_id=$2::text WHERE id=$1 AND status='pending'`, m.id, winner); err != nil {
					return err
				}
				changed = true
			}
		}
		if !changed {
			return nil
		}
	}
}

func doubleEliminationSource(ctx context.Context, tx pgx.Tx, sourceID, targetID string, slot int) (*string, bool, error) {
	var status string
	var winner *string
	var nextWinner, nextLoser *string
	var winnerSlot, loserSlot *int
	err := tx.QueryRow(ctx, `SELECT status,winner_team_id,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot FROM tournament_matches WHERE id=$1`, sourceID).Scan(&status, &winner, &nextWinner, &winnerSlot, &nextLoser, &loserSlot)
	if err != nil {
		return nil, false, err
	}
	if status == "pending" {
		return nil, false, nil
	}
	if nextWinner != nil && *nextWinner == targetID && winnerSlot != nil && *winnerSlot == slot {
		return winner, true, nil
	}
	if nextLoser != nil && *nextLoser == targetID && loserSlot != nil && *loserSlot == slot {
		if status != "complete" {
			return nil, true, nil
		}
		var a, b string
		if err = tx.QueryRow(ctx, `SELECT team_a,team_b FROM tournament_matches WHERE id=$1`, sourceID).Scan(&a, &b); err != nil {
			return nil, false, err
		}
		if winner == nil {
			return nil, true, nil
		}
		if a != *winner {
			return &a, true, nil
		}
		return &b, true, nil
	}
	return nil, true, nil
}
