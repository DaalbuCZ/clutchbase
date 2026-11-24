package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPlayerRoomCreatorEditsUseCanonicalSettingsAndPermissions(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const owner = "76561198154367261"
	const other = "76561198154367262"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	tokens := map[string]string{owner: strings.Repeat("a", 52), other: strings.Repeat("b", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO machines(id,name,address,token_hash,driver) VALUES('machine','Machine','localhost','agent','managed'); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,whitelist,allow_player_rooms) VALUES('host','Host','localhost',27015,'','machine','managed',ARRAY['76561198154367261','76561198154367262']::text[],true); INSERT INTO match_rooms(id,host_id,title,map,players_per_side,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',2,'76561198154367261'); INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','ct',true)`); err != nil {
		t.Fatal(err)
	}
	for id, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), id); err != nil {
			t.Fatal(err)
		}
	}
	call := func(id, origin, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/player/matches/"+room+"/settings", strings.NewReader(body))
		if id != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	body := `{"settingsRevision":1,"title":"Friends","map":"de_mirage","playersPerSide":2,"overtime":false}`
	if got := call("", a.Origin, body).Code; got != 401 {
		t.Fatalf("anonymous %d", got)
	}
	if got := call(owner, "https://attacker.invalid", body).Code; got != 403 {
		t.Fatalf("origin %d", got)
	}
	if got := call(other, a.Origin, body).Code; got != 404 {
		t.Fatalf("noncreator %d", got)
	}
	if got := call(owner, a.Origin, `{"settingsRevision":1,"title":"Friends","map":"de_mirage","playersPerSide":1,"overtime":false}`).Code; got != 409 {
		t.Fatalf("invalid shrink %d", got)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	stream := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + tokens[owner]}, "Origin": {a.Origin}})
	defer stream.CloseNow()
	var view playerWorkspace
	readLive(t, stream, &view)
	if len(view.Matches) != 1 || !view.Matches[0].Actions.Edit || view.Matches[0].SettingsRevision != 1 {
		t.Fatalf("missing edit action %+v", view.Matches)
	}
	if got := call(owner, a.Origin, `{"settingsRevision":1,"title":"Friday","map":"de_mirage","playersPerSide":2,"overtime":false}`); got.Code != 200 || !strings.Contains(got.Body.String(), `"readinessReset":false`) {
		t.Fatalf("title edit %d %s", got.Code, got.Body.String())
	}
	readLive(t, stream, &view)
	if view.Matches[0].Ready != 2 || view.Matches[0].SettingsRevision != 2 || view.Matches[0].Title != "Friday" {
		t.Fatalf("title update %+v", view.Matches)
	}
	if got := call(owner, a.Origin, body).Code; got != 409 {
		t.Fatalf("stale edit %d", got)
	}
	if got := call(owner, a.Origin, `{"settingsRevision":2,"title":"Friday","map":"de_nuke","playersPerSide":2,"overtime":true}`); got.Code != 200 || !strings.Contains(got.Body.String(), `"readinessReset":true`) {
		t.Fatalf("rule edit %d %s", got.Code, got.Body.String())
	}
	readLive(t, stream, &view)
	if view.Matches[0].Ready != 0 || view.Matches[0].SettingsRevision != 3 || view.Matches[0].Map != "de_nuke" || !view.Matches[0].Overtime {
		t.Fatalf("rule update %+v", view.Matches)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	view = playerWorkspace{}
	readLive(t, stream, &view)
	if view.Matches[0].Actions.Edit || view.Matches[0].SettingsRevision != 0 {
		t.Fatalf("revoked edit exposed %+v", view.Matches)
	}
	if got := call(owner, a.Origin, `{"settingsRevision":3,"title":"Friday","map":"de_nuke","playersPerSide":2,"overtime":true}`).Code; got != 404 {
		t.Fatalf("revoked edit %d", got)
	}
}

func TestPlayerRoomEditRechecksPermissionAfterHostLock(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	token := strings.Repeat("a", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'); INSERT INTO hosts(id,name,address,port,token_hash,whitelist,allow_player_rooms) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261']::text[],true); INSERT INTO match_rooms(id,host_id,title,map,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage','76561198154367261'); INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,'76561198154367261',now()+interval '1 hour')`, digest(token)); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM hosts WHERE id='host' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("PATCH", "/api/player/matches/"+room+"/settings", strings.NewReader(`{"settingsRevision":1,"title":"Changed","map":"de_mirage","playersPerSide":5,"overtime":false}`))
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
		req.Header.Set("Origin", a.Origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT h.id FROM hosts h JOIN match_rooms%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("edit did not wait for host lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got != 404 {
			t.Fatalf("revoked edit accepted %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("edit did not finish")
	}
	var title string
	if err = a.DB.QueryRow(ctx, `SELECT title FROM match_rooms WHERE id=$1`, room).Scan(&title); err != nil || title != "Friends" {
		t.Fatalf("title changed: %q %v", title, err)
	}
}
