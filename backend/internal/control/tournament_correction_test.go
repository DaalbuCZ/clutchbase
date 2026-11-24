package control

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRoundRobinCorrectionRevisionAndStandings(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format,winner_team_id) VALUES('cup','Cup',1,'complete','round_robin','a'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','winner'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status) VALUES('fixture','cup',1,1,'a','b','a','complete')`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(body string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/fixture/correction", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call(`{"teamId":"b","revision":0}`); got != 204 {
		t.Fatalf("correction status %d", got)
	}
	if got := call(`{"teamId":"a","revision":0}`); got != 409 {
		t.Fatalf("stale status %d", got)
	}
	var winner, aStatus, bStatus string
	var history int
	if err := a.DB.QueryRow(ctx, `SELECT winner_team_id,(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='b'),(SELECT count(*) FROM tournament_winner_history WHERE match_id='fixture') FROM tournaments WHERE id='cup'`).Scan(&winner, &aStatus, &bStatus, &history); err != nil || winner != "b" || aStatus != "eliminated" || bStatus != "winner" || history != 1 {
		t.Fatalf("winner %s entries %s/%s history %d: %v", winner, aStatus, bStatus, history, err)
	}
	tournaments, err := a.tournaments(ctx)
	if err != nil || len(tournaments) != 1 || len(tournaments[0].Matches) != 1 || !tournaments[0].Matches[0].WinnerCorrected {
		t.Fatalf("correction missing from snapshot: %+v %v", tournaments, err)
	}
	if history := tournaments[0].Matches[0].WinnerHistory; len(history) != 1 || history[0].PreviousWinner != "A" || history[0].NewWinner != "B" {
		t.Fatalf("winner history: %+v", history)
	}
	public := projectPlayerTournaments(tournaments, "76561198154367261", nil)
	if len(public[0].Matches[0].WinnerHistory) != 0 || !public[0].Matches[0].WinnerCorrected {
		t.Fatal("player projection exposed history or lost correction label")
	}
}

func TestConcurrentWinnerCorrectionsSerialize(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format,winner_team_id) VALUES('cup','Cup',1,'complete','round_robin','a'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','winner'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status) VALUES('fixture','cup',1,1,'a','b','a','complete')`)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/fixture/correction", bytes.NewBufferString(`{"teamId":"b","revision":0}`))
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
			rec := httptest.NewRecorder()
			a.Handler().ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for status := range results {
		counts[status]++
	}
	if counts[204] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent results: %v", counts)
	}
	var count int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM tournament_winner_history WHERE match_id='fixture'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("history count %d: %v", count, err)
	}
}

func TestEliminationCorrectionOnlyTerminalFixture(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format,winner_team_id) VALUES('cup','Cup',1,'complete','single_elimination','a'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','winner'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status) VALUES('earlier','cup',1,1,'a','b','a','complete'),('final','cup',2,1,'a','b','a','complete')`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(id string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/"+id+"/correction", bytes.NewBufferString(`{"teamId":"b","revision":0}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call("earlier"); got != 409 {
		t.Fatalf("earlier status %d", got)
	}
	if got := call("final"); got != 204 {
		t.Fatalf("final status %d", got)
	}
	var winner string
	if err := a.DB.QueryRow(ctx, `SELECT winner_team_id FROM tournaments WHERE id='cup'`).Scan(&winner); err != nil || winner != "b" {
		t.Fatalf("winner %s: %v", winner, err)
	}
}

func TestDoubleEliminationResetCorrectionProtectsGrandFinal(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format,winner_team_id) VALUES('cup','Cup',1,'complete','double_elimination','a'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','winner'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,bracket_stage) VALUES('grand','cup',1,1,'a','b','b','complete','grand_final'),('reset','cup',2,1,'b','a','a','complete','reset_final')`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(id, body string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/"+id+"/correction", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call("grand", `{"teamId":"a","revision":0}`); got != 409 {
		t.Fatalf("grand status %d", got)
	}
	if got := call("reset", `{"teamId":"b","revision":0}`); got != 204 {
		t.Fatalf("reset status %d", got)
	}
	var winner, first, second string
	if err := a.DB.QueryRow(ctx, `SELECT winner_team_id,(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='b') FROM tournaments WHERE id='cup'`).Scan(&winner, &first, &second); err != nil || winner != "b" || first != "eliminated" || second != "winner" {
		t.Fatalf("winner %s entries %s/%s: %v", winner, first, second, err)
	}
}

func TestGrandFinalCorrectionReopensUntouchedReset(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format,winner_team_id) VALUES('cup','Cup',1,'complete','double_elimination','a'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','winner'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,bracket_stage) VALUES('grand','cup',1,1,'a','b','a','complete','grand_final'); INSERT INTO tournament_matches(id,tournament_id,round,number,status,bracket_stage) VALUES('reset','cup',2,1,'skipped','reset_final'); UPDATE tournament_matches SET winner_next_match_id='reset' WHERE id='grand'`)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var canCorrect bool
	for _, m := range snapshots[0].Matches {
		if m.ID == "grand" {
			canCorrect = m.CanCorrectWinner
		}
	}
	if !canCorrect {
		t.Fatal("untouched reset should allow grand final correction")
	}
	call := func(body string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/grand/correction", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=1 WHERE id='reset'`); err != nil {
		t.Fatal(err)
	}
	if got := call(`{"teamId":"b","revision":0}`); got != 409 {
		t.Fatalf("changed reset status %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=0 WHERE id='reset'`); err != nil {
		t.Fatal(err)
	}
	if got := call(`{"teamId":"b","revision":0}`); got != 204 {
		t.Fatalf("correction status %d", got)
	}
	if got := call(`{"teamId":"a","revision":0}`); got != 409 {
		t.Fatalf("stale correction status %d", got)
	}
	var status, resetStatus, first, second, entryA, entryB string
	var winner *string
	if err := a.DB.QueryRow(ctx, `SELECT status,winner_team_id,(SELECT status FROM tournament_matches WHERE id='reset'),(SELECT team_a FROM tournament_matches WHERE id='reset'),(SELECT team_b FROM tournament_matches WHERE id='reset'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='b') FROM tournaments WHERE id='cup'`).Scan(&status, &winner, &resetStatus, &first, &second, &entryA, &entryB); err != nil || status != "active" || winner != nil || resetStatus != "pending" || first != "b" || second != "a" || entryA != "checked_in" || entryB != "checked_in" {
		t.Fatalf("reopened %s %v reset %s %s/%s entries %s/%s: %v", status, winner, resetStatus, first, second, entryA, entryB, err)
	}
	snapshots, err = a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	canCorrect = false
	for _, m := range snapshots[0].Matches {
		if m.ID == "grand" {
			canCorrect = m.CanCorrectWinner
		}
	}
	if !canCorrect {
		t.Fatal("unstarted reset should allow reverse correction")
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=1 WHERE id='reset'`); err != nil {
		t.Fatal(err)
	}
	if got := call(`{"teamId":"a","revision":1}`); got != 409 {
		t.Fatalf("started reset status %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=0 WHERE id='reset'`); err != nil {
		t.Fatal(err)
	}
	if got := call(`{"teamId":"a","revision":1}`); got != 204 {
		t.Fatalf("reverse correction status %d", got)
	}
	var history int
	if err := a.DB.QueryRow(ctx, `SELECT status,winner_team_id,(SELECT status FROM tournament_matches WHERE id='reset'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='b'),(SELECT count(*) FROM tournament_winner_history WHERE match_id='grand') FROM tournaments WHERE id='cup'`).Scan(&status, &winner, &resetStatus, &entryA, &entryB, &history); err != nil || status != "complete" || winner == nil || *winner != "a" || resetStatus != "skipped" || entryA != "winner" || entryB != "eliminated" || history != 2 {
		t.Fatalf("finished %s %v reset %s entries %s/%s history %d: %v", status, winner, resetStatus, entryA, entryB, history, err)
	}
}

func TestSingleEliminationCorrectionProtectsNextFixture(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format) VALUES('cup','Cup',1,'active','single_elimination'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','checked_in'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status) VALUES('earlier','cup',1,1,'a','b','a','complete'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,status) VALUES('next','cup',2,1,'a','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(body string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/earlier/correction", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	snapshots, err := a.tournaments(ctx)
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("snapshot: %+v %v", snapshots, err)
	}
	for _, fixture := range snapshots[0].Matches {
		if fixture.ID == "earlier" && !fixture.CanCorrectWinner {
			t.Fatal("unstarted next fixture should allow correction")
		}
	}
	public := projectPlayerTournaments(snapshots, "76561198154367261", nil)
	for _, fixture := range public[0].Matches {
		if fixture.CanCorrectWinner {
			t.Fatal("player received administrator correction action")
		}
	}
	if got := call(`{"teamId":"b","revision":0}`); got != 204 {
		t.Fatalf("correction status %d", got)
	}
	var slot string
	if err := a.DB.QueryRow(ctx, `SELECT team_a FROM tournament_matches WHERE id='next'`).Scan(&slot); err != nil || slot != "b" {
		t.Fatalf("slot %s: %v", slot, err)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=1 WHERE id='next'`); err != nil {
		t.Fatal(err)
	}
	snapshots, err = a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range snapshots[0].Matches {
		if fixture.ID == "earlier" && fixture.CanCorrectWinner {
			t.Fatal("started veto must block correction")
		}
	}
	if got := call(`{"teamId":"a","revision":1}`); got != 409 {
		t.Fatalf("started veto status %d", got)
	}
}

func TestDoubleEliminationCorrectionRequiresUntouchedRoutes(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format) VALUES('cup','Cup',1,'active','double_elimination'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','checked_in'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,bracket_stage) VALUES('upper','cup',2,1,'a',NULL,'pending','winners'),('lower','cup',3,1,'b',NULL,'pending','losers'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,bracket_stage,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot) VALUES('source','cup',1,1,'a','b','a','complete','winners','upper',1,'lower',1)`)
	if err != nil {
		t.Fatal(err)
	}
	call := func(body string) int {
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/source/correction", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		rec := httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	checkAction := func(want bool) {
		t.Helper()
		tournaments, err := a.tournaments(ctx)
		if err != nil || len(tournaments) != 1 {
			t.Fatalf("snapshot: %+v %v", tournaments, err)
		}
		for _, match := range tournaments[0].Matches {
			if match.ID == "source" {
				if match.CanCorrectWinner != want {
					t.Fatalf("correction action %v, want %v", match.CanCorrectWinner, want)
				}
				return
			}
		}
		t.Fatal("source fixture missing")
	}
	checkAction(true)
	if _, err = a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=1 WHERE id='lower'`); err != nil {
		t.Fatal(err)
	}
	checkAction(false)
	if got := call(`{"teamId":"b","revision":0}`); got != 409 {
		t.Fatalf("started veto status %d", got)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=0 WHERE id='lower'`); err != nil {
		t.Fatal(err)
	}
	if got := call(`{"teamId":"b","revision":0}`); got != 204 {
		t.Fatalf("correction status %d", got)
	}
	var upper, lower string
	if err := a.DB.QueryRow(ctx, `SELECT (SELECT team_a FROM tournament_matches WHERE id='upper'),(SELECT team_a FROM tournament_matches WHERE id='lower')`).Scan(&upper, &lower); err != nil || upper != "b" || lower != "a" {
		t.Fatalf("routes %s/%s: %v", upper, lower, err)
	}
}

func TestLosersBracketCorrectionSwapsElimination(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('a','A','76561198154367261'),('b','B','76561198154367262'); INSERT INTO tournaments(id,name,players_per_side,status,format) VALUES('cup','Cup',1,'active','double_elimination'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','eliminated'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,status,bracket_stage) VALUES('next','cup',3,1,'a','pending','losers'); INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,bracket_stage,winner_next_match_id,winner_next_slot) VALUES('source','cup',2,1,'a','b','a','complete','losers','next',1)`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range snapshot[0].Matches {
		if m.ID == "source" {
			found = m.CanCorrectWinner
		}
	}
	if !found {
		t.Fatal("correction missing from snapshot")
	}
	req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/source/correction", bytes.NewBufferString(`{"teamId":"b","revision":0}`))
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("correction status %d: %s", rec.Code, rec.Body.String())
	}
	var slot, as, bs string
	if err := a.DB.QueryRow(ctx, `SELECT (SELECT team_a FROM tournament_matches WHERE id='next'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),(SELECT status FROM tournament_entries WHERE tournament_id='cup' AND team_id='b')`).Scan(&slot, &as, &bs); err != nil || slot != "b" || as != "eliminated" || bs != "checked_in" {
		t.Fatalf("slot/status %s/%s/%s: %v", slot, as, bs, err)
	}
}
