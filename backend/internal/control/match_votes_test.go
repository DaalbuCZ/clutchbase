package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStandaloneMapVotingPermissionsAndSnapshots(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const first = "76561198154367261"
	const second = "76561198154367262"
	const outsider = "76561198154367263"
	tokens := map[string]string{first: strings.Repeat("a", 52), second: strings.Repeat("b", 52), outsider: strings.Repeat("c", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'),('76561198154367263'); INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261','76561198154367262','76561198154367263']::text[]); INSERT INTO match_rooms(id,host_id,title,map,players_per_side,manual_ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',2,true); INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',true)`); err != nil {
		t.Fatal(err)
	}
	for steam, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
			t.Fatal(err)
		}
	}
	call := func(steam, method, origin, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, "/api/player/matches/"+room+"/map-vote", strings.NewReader(body))
		if steam != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[steam]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := call("", "PUT", a.Origin, `{"map":"de_nuke"}`); got != 401 {
		t.Fatalf("no session: %d", got)
	}
	if got := call(first, "PUT", "https://attacker.invalid", `{"map":"de_nuke"}`); got != 403 {
		t.Fatalf("bad origin: %d", got)
	}
	if got := call(first, "PUT", a.Origin, `{"map":"de_nuke;quit"}`); got != 400 {
		t.Fatalf("invalid map: %d", got)
	}
	if got := call(outsider, "PUT", a.Origin, `{"map":"de_nuke"}`); got != 409 {
		t.Fatalf("nonparticipant: %d", got)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + tokens[first]}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var before playerWorkspace
	readLive(t, c, &before)
	if len(before.Matches) != 1 || !before.Matches[0].Actions.VoteMap {
		t.Fatalf("no vote action: %+v", before.Matches)
	}
	if got := call(first, "PUT", a.Origin, `{"map":"de_nuke"}`); got != 204 {
		t.Fatalf("vote: %d", got)
	}
	var after playerWorkspace
	readLive(t, c, &after)
	if len(after.Matches) != 1 || after.Matches[0].MyMapVote != "de_nuke" || len(after.Matches[0].MapVotes) != 1 || after.Matches[0].MapVotes[0].Votes != 1 || after.Matches[0].Ready != 2 {
		t.Fatalf("bad live vote %+v", after.Matches)
	}
	if got := call(second, "PUT", a.Origin, `{"map":"de_nuke"}`); got != 204 {
		t.Fatalf("second vote: %d", got)
	}
	admin, err := a.matches(ctx)
	if err != nil || len(admin) != 1 || len(admin[0].MapVotes) != 1 || admin[0].MapVotes[0].Votes != 2 {
		t.Fatalf("bad admin votes %+v %v", admin, err)
	}
	req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/settings", strings.NewReader(`{"title":"Friends","map":"de_nuke","overtime":false,"playersPerSide":2,"settingsRevision":1}`))
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("apply map: %d %s", w.Code, w.Body.String())
	}
	admin, err = a.matches(ctx)
	if err != nil || admin[0].Map != "de_nuke" || admin[0].Ready != 0 || admin[0].SettingsRevision != 2 || !admin[0].ManualReady {
		t.Fatalf("map change %+v %v", admin, err)
	}
	if got := call(first, "DELETE", a.Origin, ""); got != 204 {
		t.Fatalf("clear vote %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[$1]::text[] WHERE id='host'`, first); err != nil {
		t.Fatal(err)
	}
	admin, err = a.matches(ctx)
	if err != nil || len(admin[0].MapVotes) != 0 {
		t.Fatalf("revoked vote visible %+v %v", admin, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[$1,$2]::text[] WHERE id='host'`, first, second); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `DELETE FROM match_room_players WHERE match_id=$1 AND steam_id=$2`, room, second); err != nil {
		t.Fatal(err)
	}
	admin, err = a.matches(ctx)
	if err != nil || len(admin[0].MapVotes) != 0 {
		t.Fatalf("departed player's vote retained %+v %v", admin, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET load_status='pending' WHERE id=$1`, room); err != nil {
		t.Fatal(err)
	}
	if got := call(first, "PUT", a.Origin, `{"map":"de_mirage"}`); got != 409 {
		t.Fatalf("voted after loading %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET load_status='' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'; INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'active'); INSERT INTO tournament_matches(id,tournament_id,round,number,status,room_id) VALUES('fixture','cup',1,1,'pending','ABCDEFGHIJKLMNOPQRSTUVWX23')`); err != nil {
		t.Fatal(err)
	}
	view, err := a.playerMatches(ctx, first)
	if err != nil || len(view) != 1 || view[0].Actions.VoteMap {
		t.Fatalf("tournament voting offered %+v %v", view, err)
	}
	if got := call(first, "PUT", a.Origin, `{"map":"de_mirage"}`); got != 409 {
		t.Fatalf("tournament vote %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[]::text[] WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if got := call(first, "PUT", a.Origin, `{"map":"de_mirage"}`); got != 404 {
		t.Fatalf("revoked access %d", got)
	}
}

func TestMapVoteConcurrentWithLeaving(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	token := strings.Repeat("a", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'); INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261']); INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage'); INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, v := range []struct{ method, path, body string }{{"PUT", "map-vote", `{"map":"de_nuke"}`}, {"POST", "leave", ""}} {
		go func(v struct{ method, path, body string }) {
			<-start
			req := httptest.NewRequest(v.method, "/api/player/matches/"+room+"/"+v.path, strings.NewReader(v.body))
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
			req.Header.Set("Origin", a.Origin)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, req)
			results <- w.Code
		}(v)
	}
	close(start)
	aCode, bCode := <-results, <-results
	if !((aCode == 204 && (bCode == 204 || bCode == 409)) || (bCode == 204 && (aCode == 204 || aCode == 409))) {
		t.Fatalf("unexpected outcomes: %d %d", aCode, bCode)
	}
	var votes, players int
	if err := a.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM match_map_votes WHERE match_id=$1),(SELECT count(*) FROM match_room_players WHERE match_id=$1)`, room).Scan(&votes, &players); err != nil || votes != 0 || players != 0 {
		t.Fatalf("departure left vote behind: votes %d players %d err %v", votes, players, err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,'ct')`, room, steam); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, room); err != nil {
		t.Fatal(err)
	}
	waitingVote := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("PUT", "/api/player/matches/"+room+"/map-vote", strings.NewReader(`{"map":"de_nuke"}`))
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
		req.Header.Set("Origin", a.Origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		waitingVote <- w.Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM match_rooms WHERE id=%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("vote did not wait for room lock")
	}
	if _, err = tx.Exec(ctx, `DELETE FROM match_room_players WHERE match_id=$1 AND steam_id=$2`, room, steam); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-waitingVote:
		if code != 409 {
			t.Fatalf("vote after departure returned %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("vote did not finish")
	}
}
