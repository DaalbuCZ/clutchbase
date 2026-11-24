package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestDoubleEliminationPostgresBrackets(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	for _, entrants := range []int{3, 4, 5, 8} {
		t.Run(fmt.Sprintf("%d teams", entrants), func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"name": fmt.Sprintf("Double %d", entrants), "playersPerSide": 1, "format": "double_elimination"})
			create := httptest.NewRequest("POST", "/api/tournaments", bytes.NewReader(body))
			create.Header.Set("Authorization", "Bearer "+a.AdminToken)
			create.Header.Set("Origin", a.Origin)
			created := httptest.NewRecorder()
			a.Handler().ServeHTTP(created, create)
			if created.Code != 201 {
				t.Fatalf("create double elimination: %d %s", created.Code, created.Body.String())
			}
			var result struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			id := result.ID
			if id == "" {
				t.Fatal("created tournament has no id")
			}
			if _, err := a.DB.Exec(ctx, `UPDATE tournaments SET status='checkin' WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < entrants; i++ {
				steam, team := rand.Text(), fmt.Sprintf("%s-%02d", id, i)
				if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, steam); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, team, steam); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'checked_in')`, id, team); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest("POST", "/api/tournaments/"+id+"/bracket", nil)
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, req)
			if w.Code != 204 {
				t.Fatalf("generate bracket: %d %s", w.Code, w.Body.String())
			}
			size := 1
			for size < entrants {
				size *= 2
			}
			var total, byeWinners, unsettled int
			if err := a.DB.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status='bye' AND winner_team_id IS NOT NULL),count(*) FILTER(WHERE status='pending' AND bracket_stage IN ('winners','losers') AND ((source_a_match_id IS NULL AND team_a IS NULL AND source_b_match_id IS NULL AND team_b IS NOT NULL) OR (source_b_match_id IS NULL AND team_b IS NULL AND source_a_match_id IS NULL AND team_a IS NOT NULL))) FROM tournament_matches WHERE tournament_id=$1`, id).Scan(&total, &byeWinners, &unsettled); err != nil {
				t.Fatal(err)
			}
			if total != 2*size-1 || unsettled != 0 {
				t.Fatalf("bad graph: total=%d unsettled=%d", total, unsettled)
			}
			if entrants < size && byeWinners == 0 {
				t.Fatalf("%d entrants have no automatic bye", entrants)
			}
		})
	}
}

func TestDoubleEliminationPostgresResetEliminationAndRace(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := rand.Text()
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status,format) VALUES($1,'Reset cup',1,'checkin','double_elimination')`, id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		steam, team := rand.Text(), fmt.Sprintf("%s-%d", id, i)
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, steam); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, team, steam); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'checked_in')`, id, team); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournaments SET status='active' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	matches := doubleEliminationBracket(registeredTeamIDs(t, ctx, a, id))
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, m := range matches {
		var aa, bb, winnerNext, winnerSlot, loserNext, loserSlot, sourceA, sourceB any
		if m.a != "" {
			aa = m.a
		}
		if m.b != "" {
			bb = m.b
		}
		if m.winnerMatch != "" {
			winnerNext = m.winnerMatch
			winnerSlot = m.winnerSlot
		}
		if m.loserMatch != "" {
			loserNext = m.loserMatch
			loserSlot = m.loserSlot
		}
		if m.sourceA != "" {
			sourceA = m.sourceA
		}
		if m.sourceB != "" {
			sourceB = m.sourceB
		}
		if _, err := tx.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,bracket_stage,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot,source_a_match_id,source_b_match_id) VALUES($1,$2,$3,$4,$5,$6,'pending',$7,$8,$9,$10,$11,$12,$13)`, m.id, id, m.round, m.number, aa, bb, m.stage, winnerNext, winnerSlot, loserNext, loserSlot, sourceA, sourceB); err != nil {
			t.Fatal(err)
		}
	}
	if err = settleDoubleEliminationByes(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for {
		var matchID, team string
		err = a.DB.QueryRow(ctx, `SELECT id,team_a FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage IN ('winners','losers') AND status='pending' AND team_a IS NOT NULL AND team_b IS NOT NULL ORDER BY round,number LIMIT 1`, id).Scan(&matchID, &team)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tx, err = a.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = advanceTournamentWinner(ctx, tx, id, matchID, 0, 0, team); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var grandID, lowerTeam string
	if err = a.DB.QueryRow(ctx, `SELECT id,team_b FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage='grand_final' AND team_a IS NOT NULL AND team_b IS NOT NULL`, id).Scan(&grandID, &lowerTeam); err != nil {
		t.Fatal(err)
	}
	tx, err = a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = advanceTournamentWinner(ctx, tx, id, grandID, 0, 0, lowerTeam); err != nil {
		tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var resetID, resetA, resetB string
	if err = a.DB.QueryRow(ctx, `SELECT id,team_a,team_b FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage='reset_final'`, id).Scan(&resetID, &resetA, &resetB); err != nil {
		t.Fatal(err)
	}
	if resetA == "" || resetB == "" || resetA == resetB {
		t.Fatalf("reset final not populated: %q/%q", resetA, resetB)
	}
	var eliminated int
	if err = a.DB.QueryRow(ctx, `SELECT count(*) FROM tournament_entries WHERE tournament_id=$1 AND status='eliminated'`, id).Scan(&eliminated); err != nil {
		t.Fatal(err)
	}
	if eliminated == 0 {
		t.Fatal("second loss did not eliminate a team")
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, winner := range []string{resetA, resetB} {
		wg.Add(1)
		go func(winner string) {
			defer wg.Done()
			<-start
			tx, err := a.DB.Begin(ctx)
			if err != nil {
				errs <- err
				return
			}
			err = advanceTournamentWinner(ctx, tx, id, resetID, 0, 0, winner)
			if err == nil {
				err = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			errs <- err
		}(winner)
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("race accepted %d winners", success)
	}
	var status, winner string
	if err = a.DB.QueryRow(ctx, `SELECT status,winner_team_id FROM tournaments WHERE id=$1`, id).Scan(&status, &winner); err != nil {
		t.Fatal(err)
	}
	if status != "complete" || winner == "" {
		t.Fatalf("reset did not complete tournament: %s/%s", status, winner)
	}
}

func registeredTeamIDs(t *testing.T, ctx context.Context, a *API, id string) []string {
	t.Helper()
	rows, err := a.DB.Query(ctx, `SELECT team_id FROM tournament_entries WHERE tournament_id=$1 ORDER BY team_id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var team string
		if err = rows.Scan(&team); err != nil {
			t.Fatal(err)
		}
		out = append(out, team)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDoubleEliminationPostgresSkipsResetWhenWinnersChampionWins(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := rand.Text()
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status,format) VALUES($1,'No reset cup',1,'checkin','double_elimination')`, id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		steam, team := rand.Text(), fmt.Sprintf("%s-%d", id, i)
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, steam); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, team, steam); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'checked_in')`, id, team); err != nil {
			t.Fatal(err)
		}
	}
	build := httptest.NewRequest("POST", "/api/tournaments/"+id+"/bracket", nil)
	build.Header.Set("Authorization", "Bearer "+a.AdminToken)
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, build)
	if response.Code != 204 {
		t.Fatalf("build two-team double elimination bracket: %d %s", response.Code, response.Body.String())
	}
	var opening, upperChampion, grandFinal string
	if err := a.DB.QueryRow(ctx, `SELECT id,team_a FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage='winners'`, id).Scan(&opening, &upperChampion); err != nil {
		t.Fatal(err)
	}
	advance := func(matchID, winner string) {
		t.Helper()
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = advanceTournamentWinner(ctx, tx, id, matchID, 0, 0, winner); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	advance(opening, upperChampion)
	if err := a.DB.QueryRow(ctx, `SELECT id FROM tournament_matches WHERE tournament_id=$1 AND bracket_stage='grand_final'`, id).Scan(&grandFinal); err != nil {
		t.Fatal(err)
	}
	advance(grandFinal, upperChampion)
	var resetStatus, tournamentStatus string
	if err := a.DB.QueryRow(ctx, `SELECT m.status,t.status FROM tournament_matches m JOIN tournaments t ON t.id=m.tournament_id WHERE m.tournament_id=$1 AND m.bracket_stage='reset_final'`, id).Scan(&resetStatus, &tournamentStatus); err != nil {
		t.Fatal(err)
	}
	if resetStatus != "skipped" || tournamentStatus != "complete" {
		t.Fatalf("reset status=%q tournament status=%q", resetStatus, tournamentStatus)
	}
}
