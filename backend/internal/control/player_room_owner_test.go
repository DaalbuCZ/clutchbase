package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
)

func TestRoomCreatorCanLoadOnlyReadyOwnRoomWithCurrentPermission(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const owner = "76561198154367261"
	const other = "76561198154367262"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	tokens := map[string]string{owner: strings.Repeat("a", 52), other: strings.Repeat("b", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation,supports_whitelist) VALUES('machine','Machine','localhost','agent','managed',now(),true,true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,whitelist,active_preset,access_status,active_map,can_load_match,allow_player_rooms,match_phase) VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),ARRAY['76561198154367261','76561198154367262']::text[],'match','verified','de_mirage',true,true,'idle'); INSERT INTO match_rooms(id,host_id,title,map,players_per_side,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_nuke',1,'76561198154367261'); INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',false)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist_hash=$1 WHERE id='host'`, protocol.WhitelistHash([]string{owner, other})); err != nil {
		t.Fatal(err)
	}
	for id, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), id); err != nil {
			t.Fatal(err)
		}
	}
	call := func(id, action, origin string) int {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/"+action, nil)
		if id != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := call("", "load", a.Origin); got != 401 {
		t.Fatalf("anonymous load %d", got)
	}
	if got := call(owner, "load", "https://attacker.invalid"); got != 403 {
		t.Fatalf("origin %d", got)
	}
	if got := call(other, "load", a.Origin); got != 404 {
		t.Fatalf("noncreator load %d", got)
	}
	if got := call(owner, "load", a.Origin); got != 409 {
		t.Fatalf("unready room load %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[$1]::text[],whitelist_hash=$2 WHERE id='host'`, other, protocol.WhitelistHash([]string{other})); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, "load", a.Origin); got != 404 {
		t.Fatalf("removed roster access accepted %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[$1,$2]::text[],whitelist_hash=$3 WHERE id='host'`, owner, other, protocol.WhitelistHash([]string{owner, other})); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + tokens[owner]}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var view playerWorkspace
	readLive(t, c, &view)
	if len(view.Matches) != 1 || !view.Matches[0].MyRoom || view.Matches[0].Actions.Load || !slices.Contains(view.Matches[0].LoadBlockers, "players_not_ready") || !view.Matches[0].Actions.Close {
		t.Fatalf("unready actions %+v", view.Matches)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1 AND steam_id=$2`, room, other); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &view)
	if !view.Matches[0].Actions.Load || len(view.Matches[0].LoadBlockers) != 0 || !view.Matches[0].Actions.Close {
		t.Fatalf("ready actions %+v", view.Matches[0])
	}
	otherMatches, err := a.playerMatches(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	state, err := a.workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	otherView := playerWorkspace{Matches: otherMatches}
	a.projectPlayerRoomActions(&otherView, state, other)
	if len(otherView.Matches) != 1 || otherView.Matches[0].Actions.Load || !slices.Contains(otherView.Matches[0].LoadBlockers, "load_permission_required") {
		t.Fatalf("noncreator load permission not explained: %+v", otherView.Matches)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &view)
	if view.Matches[0].Actions.Load || view.Matches[0].Actions.Close {
		t.Fatalf("revoked actions %+v", view.Matches[0])
	}
	if got := call(owner, "load", a.Origin); got != 404 {
		t.Fatalf("revoked load %d", got)
	}
	if got := call(owner, "close", a.Origin); got != 404 {
		t.Fatalf("revoked close %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=true WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &view)
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, room); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() { result <- call(owner, "load", a.Origin) }()
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
		t.Fatal("load did not wait for room lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE match_room_players SET ready=false WHERE match_id=$1 AND steam_id=$2`, room, other); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got != 409 {
			t.Fatalf("stale ready state accepted %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("load did not finish")
	}
	readLive(t, c, &view)
	if view.Matches[0].Actions.Load {
		t.Fatal("unready room exposed load")
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1 AND steam_id=$2`, room, other); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &view)
	if !view.Matches[0].Actions.Load {
		t.Fatal("ready room did not restore load action")
	}
	if got := call(owner, "load", a.Origin); got != 202 {
		t.Fatalf("ready load %d", got)
	}
	readLive(t, c, &view)
	if view.Matches[0].LoadStatus != "pending" || view.Matches[0].Actions.Load || view.Matches[0].Actions.Close {
		t.Fatalf("pending actions %+v", view.Matches[0])
	}
	if got := call(owner, "close", a.Origin); got != 404 {
		t.Fatalf("pending close %d", got)
	}
	var payload []byte
	if err := a.DB.QueryRow(ctx, `SELECT load_payload FROM match_rooms WHERE id=$1`, room).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var setup protocol.MatchSetup
	if err := json.Unmarshal(payload, &setup); err != nil || len(setup.CT) != 1 || len(setup.T) != 1 || setup.CT[0].SteamID != owner || setup.T[0].SteamID != other {
		t.Fatalf("incorrect setup %+v %v", setup, err)
	}
}

func TestRoomCreatorCanCloseOnlyOwnUnstartedRoom(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const owner = "76561198154367261"
	const other = "76561198154367262"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	tokens := map[string]string{owner: strings.Repeat("a", 52), other: strings.Repeat("b", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO hosts(id,name,address,port,token_hash,whitelist,allow_player_rooms) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261','76561198154367262']::text[],true); INSERT INTO match_rooms(id,host_id,title,map,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage','76561198154367261'); INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct'),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t')`); err != nil {
		t.Fatal(err)
	}
	for id, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), id); err != nil {
			t.Fatal(err)
		}
	}
	call := func(id string) int {
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/close", nil)
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		req.Header.Set("Origin", a.Origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := call(other); got != 404 {
		t.Fatalf("noncreator close %d", got)
	}
	if got := call(owner); got != 204 {
		t.Fatalf("creator close %d", got)
	}
	var status, load string
	if err := a.DB.QueryRow(ctx, `SELECT status,load_status FROM match_rooms WHERE id=$1`, room).Scan(&status, &load); err != nil || status != "closed" || load != "" {
		t.Fatalf("close state %q %q %v", status, load, err)
	}
	if got := call(owner); got != 404 {
		t.Fatalf("replayed close %d", got)
	}
}
