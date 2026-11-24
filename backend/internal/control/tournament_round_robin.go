package control

import (
	"context"
	"sort"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

type roundRobinFixture struct {
	round, number int
	a, b          string
}

// The circle schedule gives each team at most one fixture per round.
func roundRobinFixtures(ids []string) []roundRobinFixture {
	if len(ids) < 2 {
		return []roundRobinFixture{}
	}
	teams := append([]string(nil), ids...)
	if len(teams)%2 != 0 {
		teams = append(teams, "")
	}
	fixtures := []roundRobinFixture{}
	for round := 1; round < len(teams); round++ {
		number := 0
		for i := 0; i < len(teams)/2; i++ {
			a, b := teams[i], teams[len(teams)-1-i]
			if a == "" || b == "" {
				continue
			}
			if round%2 == 0 {
				a, b = b, a
			}
			number++
			fixtures = append(fixtures, roundRobinFixture{round, number, a, b})
		}
		last := teams[len(teams)-1]
		copy(teams[2:], teams[1:len(teams)-1])
		teams[1] = last
	}
	return fixtures
}

func tournamentStandings(t protocol.Tournament) []protocol.TournamentStanding {
	out := []protocol.TournamentStanding{}
	if t.Format != "round_robin" {
		return out
	}
	byID := map[string]int{}
	for _, e := range t.Entries {
		if e.Status == "registered" {
			continue
		}
		byID[e.TeamID] = len(out)
		out = append(out, protocol.TournamentStanding{TeamID: e.TeamID, TeamName: e.TeamName})
	}
	for _, m := range t.Matches {
		if m.Status != "complete" || m.TeamA == nil || m.TeamB == nil || m.WinnerTeamID == "" {
			continue
		}
		for _, id := range []string{m.TeamA.TeamID, m.TeamB.TeamID} {
			i, ok := byID[id]
			if !ok {
				continue
			}
			out[i].Played++
			if id == m.WinnerTeamID {
				out[i].Wins++
			} else {
				out[i].Losses++
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Wins != out[j].Wins {
			return out[i].Wins > out[j].Wins
		}
		if out[i].TeamName != out[j].TeamName {
			return out[i].TeamName < out[j].TeamName
		}
		return out[i].TeamID < out[j].TeamID
	})
	for i := range out {
		out[i].Rank = i + 1
		if i > 0 && out[i].Wins == out[i-1].Wins {
			out[i].Rank = out[i-1].Rank
		}
	}
	return out
}

func finishRoundRobin(ctx context.Context, tx pgx.Tx, id string) error {
	var pending bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_matches WHERE tournament_id=$1 AND status='pending')`, id).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT e.team_id,count(m.id)::int FROM tournament_entries e LEFT JOIN tournament_matches m ON m.tournament_id=e.tournament_id AND m.winner_team_id=e.team_id AND m.status='complete' WHERE e.tournament_id=$1 AND e.status='checked_in' GROUP BY e.team_id ORDER BY count(m.id) DESC,e.team_id`, id)
	if err != nil {
		return err
	}
	winners := []string{}
	best := -1
	for rows.Next() {
		var team string
		var wins int
		if err = rows.Scan(&team, &wins); err != nil {
			rows.Close()
			return err
		}
		if best < 0 {
			best = wins
		}
		if wins == best {
			winners = append(winners, team)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var winner any
	if len(winners) == 1 {
		winner = winners[0]
	}
	if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='complete',winner_team_id=$2 WHERE id=$1`, id, winner); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE tournament_entries SET status=CASE WHEN team_id=ANY($2::text[]) THEN 'winner' ELSE 'eliminated' END WHERE tournament_id=$1 AND status='checked_in'`, id, winners)
	return err
}
