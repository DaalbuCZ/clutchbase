package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShuffleStandaloneMatchPreservesRosterAndResetsReadiness(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,''); INSERT INTO match_rooms(id,host_id,title,map,players_per_side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',2); INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'),('76561198154367263'); INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367263','t',true)`); err != nil {
		t.Fatal(err)
	}
	current := []shufflePlayer{{"76561198154367261", "ct", true}, {"76561198154367262", "ct", true}, {"76561198154367263", "t", true}}
	token := strings.Repeat("p", 52)
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY['76561198154367261']::text[] WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,'76561198154367261',now()+interval '1 hour')`, digest(token)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var public playerWorkspace
	readLive(t, c, &public)
	if len(public.Matches) != 1 || public.Matches[0].Ready != 3 {
		t.Fatalf("initial player roster %+v", public.Matches)
	}
	call := func(token string, rev int, players []shufflePlayer) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"settingsRevision": rev, "players": players})
		req := httptest.NewRequest("POST", "/api/matches/"+room+"/shuffle", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := call("wrong", 1, current); code != 401 {
		t.Fatalf("unauthorized %d", code)
	}
	if code := call(a.AdminToken, 2, current); code != 409 {
		t.Fatalf("stale settings %d", code)
	}
	stale := append([]shufflePlayer(nil), current...)
	stale[0].Ready = false
	if code := call(a.AdminToken, 1, stale); code != 409 {
		t.Fatalf("stale ready state %d", code)
	}
	bad := append([]shufflePlayer(nil), current...)
	bad[1] = bad[0]
	if code := call(a.AdminToken, 1, bad); code != 400 {
		t.Fatalf("duplicate roster %d", code)
	}
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || !rooms[0].CanShuffle {
		t.Fatalf("missing shuffle action %+v %v", rooms, err)
	}
	if code := call(a.AdminToken, 1, current); code != 204 {
		t.Fatalf("shuffle %d", code)
	}
	readLive(t, c, &public)
	if len(public.Matches) != 1 || public.Matches[0].Ready != 0 || public.Matches[0].Players != 3 {
		t.Fatalf("player did not receive shuffle %+v", public.Matches)
	}
	rooms, err = a.matches(ctx)
	if err != nil || len(rooms) != 1 || rooms[0].Players != 3 || rooms[0].Ready != 0 || rooms[0].CTPlayers < 1 || rooms[0].CTPlayers > 2 || rooms[0].TPlayers < 1 || rooms[0].TPlayers > 2 {
		t.Fatalf("invalid result %+v %v", rooms, err)
	}
	for _, p := range rooms[0].Participants {
		if p.Ready {
			t.Fatal("readiness not reset")
		}
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_rooms SET load_status='pending' WHERE id=$1`, room); err != nil {
		t.Fatal(err)
	}
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CanShuffle {
		t.Fatalf("shuffle exposed after load %+v %v", rooms, err)
	}
	if code := call(a.AdminToken, 1, current); code != 409 {
		t.Fatalf("loaded match accepted %d", code)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_rooms SET load_status='' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'; INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'active'); INSERT INTO tournament_matches(id,tournament_id,round,number,status,room_id) VALUES('fixture','cup',1,1,'pending','ABCDEFGHIJKLMNOPQRSTUVWX23')`); err != nil {
		t.Fatal(err)
	}
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CanShuffle {
		t.Fatalf("tournament action exposed %+v %v", rooms, err)
	}
	if code := call(a.AdminToken, 1, current); code != 409 {
		t.Fatalf("tournament accepted %d", code)
	}
}

func TestShuffleRechecksRosterAfterRoomLock(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,''); INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage'); INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct'),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t')`); err != nil {
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
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("POST", "/api/matches/"+room+"/shuffle", strings.NewReader(`{"settingsRevision":1,"players":[{"steamId":"76561198154367261","side":"ct","ready":false},{"steamId":"76561198154367262","side":"t","ready":false}]}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT m.id FROM match_rooms m JOIN hosts%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("shuffle did not wait for room lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1 AND steam_id='76561198154367261'`, room); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != 409 {
			t.Fatalf("stale roster accepted %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shuffle did not finish")
	}
	var ready bool
	if err = a.DB.QueryRow(ctx, `SELECT ready FROM match_room_players WHERE match_id=$1 AND steam_id='76561198154367261'`, room).Scan(&ready); err != nil || !ready {
		t.Fatalf("changed readiness overwritten: %v %v", ready, err)
	}
}
