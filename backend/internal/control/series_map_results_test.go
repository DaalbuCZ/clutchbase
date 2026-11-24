package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSeriesMapResultsAreRevisionProtectedVisibleAndLive(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const (
		room    = "series-map-result-room"
		open    = "series-map-result-open"
		failed  = "series-map-result-failed"
		removed = "series-map-result-removed"
		hidden  = "series-map-result-hidden"
		viewer  = "76561198154367261"
		other   = "76561198154367262"
	)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Viewer'),($2,'Other')`, viewer, other)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,whitelist,removed_at) VALUES
        ('series-visible','Visible','localhost',27015,'',ARRAY[$1]::text[],NULL),
        ('series-hidden','Hidden','localhost',27025,'',ARRAY[$2]::text[],NULL),
        ('series-removed','Removed','localhost',27035,'',ARRAY[$1]::text[],now())`, viewer, other)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,series_maps,reported_ct,reported_t,reported_at,result_ct,result_t,reported_team1_score,reported_team2_score) VALUES
		($1,'series-visible','Series','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],13,9,now(),15,11,2,1),
        ($2,'series-visible','Open','de_mirage','open','complete',ARRAY['de_mirage','de_nuke','de_inferno'],NULL,NULL,NULL,NULL,NULL,NULL,NULL),
        ($3,'series-visible','Failed','de_mirage','closed','failed',ARRAY['de_mirage','de_nuke','de_inferno'],NULL,NULL,NULL,NULL,NULL,NULL,NULL),
        ($4,'series-removed','Removed','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],NULL,NULL,NULL,NULL,NULL,NULL,NULL),
        ($5,'series-hidden','Hidden','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],NULL,NULL,NULL,NULL,NULL,NULL,NULL)`, room, open, failed, removed, hidden)
	// Keep a completed tournament fixture attached to the same room. Editing a map
	// score must not rewrite either the reported series or tournament result.
	exec(`INSERT INTO teams(id,name,captain_steam_id) VALUES('series-team-a','Alpha',$1),('series-team-b','Bravo',$2)`, viewer, other)
	exec(`INSERT INTO tournaments(id,name,players_per_side,status,best_of,winner_team_id) VALUES('series-cup','Cup',1,'complete',3,'series-team-a')`)
	exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('series-cup','series-team-a','checked_in'),('series-cup','series-team-b','checked_in')`)
	exec(`INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,room_id) VALUES('series-fixture','series-cup',1,1,'series-team-a','series-team-b','series-team-a','complete',$1)`, room)

	server := httptest.NewServer(a.Handler())
	defer server.Close()
	live := liveDial(t, server.URL+"/api/live?view=history", a.AdminToken, nil)
	var initial historyPage
	readLive(t, live, &initial)
	initialMatch := seriesHistoryMatch(t, initial, room)
	if len(initialMatch.SeriesMaps) != 3 || initialMatch.SeriesMaps[0].Team1 != nil || initialMatch.SeriesMaps[1].Team1 != nil || initialMatch.SeriesMaps[2].Team1 != nil {
		t.Fatalf("initial missing-result projection: %+v", initialMatch.SeriesMaps)
	}

	call := func(id, token, body string, want int) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/matches/"+id+"/series-map-result", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", id, body, res.Code, want, res.Body.String())
		}
	}

	call(room, "wrong", `{"mapNumber":1,"team1":13,"team2":7,"revision":0}`, 401)
	for _, body := range []string{
		`{"mapNumber":0,"team1":13,"team2":7,"revision":0}`,
		`{"mapNumber":6,"team1":13,"team2":7,"revision":0}`,
		`{"mapNumber":1,"team1":100,"team2":7,"revision":0}`,
		`{"mapNumber":1,"team1":13,"team2":-1,"revision":0}`,
		`{"mapNumber":1,"team1":13,"revision":0}`,
		`{"mapNumber":1,"team1":13,"team2":7}`,
		`{"mapNumber":1,"clear":true,"team1":13,"revision":0}`,
	} {
		call(room, a.AdminToken, body, 400)
	}
	call(room, a.AdminToken, `{"mapNumber":1,"team1":13,"team2":7,"revision":0}`, 204)
	var liveAfter historyPage
	readLive(t, live, &liveAfter)
	if got := seriesHistoryMatch(t, liveAfter, room); got.SeriesMaps[0].Team1 == nil || *got.SeriesMaps[0].Team1 != 13 || got.SeriesMaps[0].Team2 == nil || *got.SeriesMaps[0].Team2 != 7 {
		t.Fatalf("live history did not publish map result: %+v", got.SeriesMaps)
	}

	// Two writers using the same snapshot cannot both commit.
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, body := range []string{
		`{"mapNumber":2,"team1":16,"team2":12,"revision":1}`,
		`{"mapNumber":2,"team1":17,"team2":11,"revision":1}`,
	} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/series-map-result", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
			res := httptest.NewRecorder()
			a.Handler().ServeHTTP(res, req)
			codes <- res.Code
		}(body)
	}
	wg.Wait()
	close(codes)
	var committed, rejected int
	for code := range codes {
		switch code {
		case 204:
			committed++
		case 409:
			rejected++
		default:
			t.Fatalf("concurrent edit returned %d", code)
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("concurrent edits committed=%d rejected=%d", committed, rejected)
	}
	readLive(t, live, &liveAfter)
	current := seriesHistoryMatch(t, liveAfter, room)
	if current.ResultRevision != 2 || current.SeriesMaps[1].Team1 == nil {
		t.Fatalf("concurrent revision/result: %+v", current)
	}

	call(room, a.AdminToken, `{"mapNumber":4,"team1":1,"team2":0,"revision":2}`, 409)
	call(room, a.AdminToken, `{"mapNumber":1,"clear":true,"revision":2}`, 204)
	history, err := a.matchHistory(ctx, viewer, "")
	if err != nil {
		t.Fatal(err)
	}
	series := seriesHistoryMatch(t, history, room)
	if series.SeriesMaps[0].Team1 != nil || series.SeriesMaps[1].Team1 == nil || series.SeriesMaps[2].Team1 != nil {
		t.Fatalf("missing configured map results not preserved: %+v", series.SeriesMaps)
	}
	if series.ReportedCT == nil || *series.ReportedCT != 13 || series.ReportedT == nil || *series.ReportedT != 9 || series.SeriesTeam1 == nil || *series.SeriesTeam1 != 2 || series.SeriesTeam2 == nil || *series.SeriesTeam2 != 1 {
		t.Fatalf("reported series changed: %+v", series)
	}
	if series.ResultCT == nil || *series.ResultCT != 15 || series.ResultT == nil || *series.ResultT != 11 || series.ResultRevision != 3 {
		t.Fatalf("final result projection: %+v", series)
	}
	if rows, err := a.tournaments(ctx); err != nil || len(rows) != 1 || rows[0].WinnerTeamID != "series-team-a" || len(rows[0].Matches) != 1 || rows[0].Matches[0].WinnerTeamID != "series-team-a" {
		t.Fatalf("tournament result changed: %+v %v", rows, err)
	}

	// Player history uses the host whitelist, and omitted map rows remain null.
	if len(history.Matches) != 1 {
		t.Fatalf("viewer saw private or removed history: %+v", history.Matches)
	}
	otherHistory, err := a.matchHistory(ctx, other, "")
	if err != nil || len(otherHistory.Matches) != 1 || otherHistory.Matches[0].ID != hidden {
		t.Fatalf("other whitelist projection: %+v %v", otherHistory, err)
	}

	call(open, a.AdminToken, `{"mapNumber":1,"team1":1,"team2":0,"revision":0}`, 409)
	call(failed, a.AdminToken, `{"mapNumber":1,"team1":1,"team2":0,"revision":0}`, 409)
	call(removed, a.AdminToken, `{"mapNumber":1,"team1":1,"team2":0,"revision":0}`, 404)
	call(hidden, a.AdminToken, `{"mapNumber":1,"team1":1,"team2":0,"revision":0}`, 204)

	// If removal commits while an edit is waiting for the host row lock, the
	// edit must re-check the host and reject the now-removed room.
	lock, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(ctx, `SELECT id FROM hosts WHERE id='series-visible' FOR UPDATE`); err != nil {
		lock.Rollback(ctx)
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/series-map-result", strings.NewReader(`{"mapNumber":3,"team1":9,"team2":8,"revision":3}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		result <- res.Code
	}()
	if _, err := lock.Exec(ctx, `UPDATE hosts SET removed_at=now() WHERE id='series-visible'`); err != nil {
		lock.Rollback(ctx)
		t.Fatal(err)
	}
	if err := lock.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got != 404 {
		t.Fatalf("edit after host removal returned %d", got)
	}
}

func seriesHistoryMatch(t *testing.T, page historyPage, id string) historyMatch {
	t.Helper()
	for _, match := range page.Matches {
		if match.ID == id {
			return match
		}
	}
	t.Fatalf("history missing match %q: %+v", id, page.Matches)
	return historyMatch{}
}
