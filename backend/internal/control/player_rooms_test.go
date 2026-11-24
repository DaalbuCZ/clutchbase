package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
)

func TestPlayerRoomCreationRequiresOptInAndRoster(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const first = "76561198154367261"
	const second = "76561198154367262"
	const outsider = "76561198154367263"
	tokens := map[string]string{first: strings.Repeat("a", 52), second: strings.Repeat("b", 52), outsider: strings.Repeat("c", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'),('76561198154367263'); INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation) VALUES('machine','Machine','localhost','agent','managed',now(),true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,whitelist,active_preset,access_status,active_map) VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),ARRAY['76561198154367261','76561198154367262']::text[],'match','verified','de_mirage')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE machines SET supports_whitelist=true WHERE id='machine'`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist_hash=$1 WHERE id='host'`, protocol.WhitelistHash([]string{first, second})); err != nil {
		t.Fatal(err)
	}
	for id, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), id); err != nil {
			t.Fatal(err)
		}
	}
	create := func(steam, origin, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/player/matches", strings.NewReader(body))
		if steam != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[steam]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	body := `{"hostId":"host","title":"Friends","playersPerSide":2,"map":"de_nuke","side":"ct","overtime":true}`
	if got := create("", a.Origin, body).Code; got != 401 {
		t.Fatalf("session %d", got)
	}
	if got := create(first, "https://attacker.invalid", body).Code; got != 403 {
		t.Fatalf("origin %d", got)
	}
	if got := create(first, a.Origin, body).Code; got != 404 {
		t.Fatalf("disabled %d", got)
	}
	state, err := a.workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if projectPlayer(state, first).Servers[0].CanOpenMatch {
		t.Fatal("disabled server offers action")
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	stream := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + tokens[first]}, "Origin": {a.Origin}})
	defer stream.CloseNow()
	var live playerWorkspace
	readLive(t, stream, &live)
	if len(live.Servers) != 1 || live.Servers[0].CanOpenMatch {
		t.Fatalf("unexpected initial action %+v", live.Servers)
	}
	change := func(body string) int {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/hosts/host", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := change(`{"revision":2,"allowPlayerRooms":true}`); got != 409 {
		t.Fatalf("stale enable %d", got)
	}
	if got := change(`{"revision":1,"allowPlayerRooms":true}`); got != 200 {
		t.Fatalf("enable %d", got)
	}
	readLive(t, stream, &live)
	if len(live.Servers) != 1 || !live.Servers[0].CanOpenMatch {
		t.Fatalf("live permission missing %+v", live.Servers)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist_hash='' WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, stream, &live)
	if live.Servers[0].CanOpenMatch {
		t.Fatal("unapplied roster allowed room creation")
	}
	if got := create(first, a.Origin, body).Code; got != 409 {
		t.Fatalf("unapplied roster accepted %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist_hash=$1 WHERE id='host'`, protocol.WhitelistHash([]string{first, second})); err != nil {
		t.Fatal(err)
	}
	readLive(t, stream, &live)
	if !live.Servers[0].CanOpenMatch {
		t.Fatal("applied roster did not restore room action")
	}
	state, err = a.workspace(ctx)
	if err != nil || !projectPlayer(state, first).Servers[0].CanOpenMatch {
		t.Fatalf("missing enabled action %+v %v", state.Hosts, err)
	}
	if got := create(outsider, a.Origin, body).Code; got != 404 {
		t.Fatalf("outsider %d", got)
	}
	if got := create(first, a.Origin, `{"hostId":"host","title":"Friends","side":"ct","map":"de_nuke;quit"}`).Code; got != 400 {
		t.Fatalf("bad map %d", got)
	}
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, id := range []string{first, second} {
		go func(id string) { <-start; results <- create(id, a.Origin, body) }(id)
	}
	close(start)
	one, two := <-results, <-results
	if !((one.Code == 201 && two.Code == 409) || (one.Code == 409 && two.Code == 201)) {
		t.Fatalf("concurrent rooms %d %d", one.Code, two.Code)
	}
	readLive(t, stream, &live)
	if len(live.Servers) != 1 || live.Servers[0].CanOpenMatch || len(live.Matches) != 1 {
		t.Fatalf("live room snapshot %+v", live)
	}
	success := one
	if two.Code == 201 {
		success = two
	}
	var output map[string]any
	if err := json.Unmarshal(success.Body.Bytes(), &output); err != nil || len(output) != 1 || output["id"] == "" {
		t.Fatalf("player response %+v %v", output, err)
	}
	var count int
	var joined, side, creator string
	var ready, overtime bool
	if err = a.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM match_rooms WHERE host_id='host'),p.steam_id,p.side,p.ready,m.overtime,COALESCE(m.created_by_steam_id,'') FROM match_rooms m JOIN match_room_players p ON p.match_id=m.id WHERE m.host_id='host'`).Scan(&count, &joined, &side, &ready, &overtime, &creator); err != nil || count != 1 || (joined != first && joined != second) || creator != joined || side != "ct" || ready || !overtime {
		t.Fatalf("creation/roster %d %q %q %v %v %v", count, joined, side, ready, overtime, err)
	}
	state, err = a.workspace(ctx)
	if err != nil || projectPlayer(state, first).Servers[0].CanOpenMatch {
		t.Fatalf("room already open %+v %v", state.Hosts, err)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed' WHERE host_id='host'`); err != nil {
		t.Fatal(err)
	}
	if got := change(`{"revision":2,"allowPlayerRooms":false}`); got != 200 {
		t.Fatalf("disable %d", got)
	}
	if got := create(first, a.Origin, body).Code; got != 404 {
		t.Fatalf("disabled again %d", got)
	}
}

func TestPlayerRoomCreationRechecksPermissionAfterHostLock(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := strings.Repeat("a", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'); INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation) VALUES('machine','Machine','localhost','agent','managed',now(),true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,whitelist,active_preset,access_status,active_map,allow_player_rooms) VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),ARRAY['76561198154367261']::text[],'match','verified','de_mirage',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE machines SET supports_whitelist=true WHERE id='machine'`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist_hash=$1 WHERE id='host'`, protocol.WhitelistHash([]string{"76561198154367261"})); err != nil {
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
		req := httptest.NewRequest("POST", "/api/player/matches", strings.NewReader(`{"hostId":"host","title":"Friends","side":"ct"}`))
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
		req.Header.Set("Origin", a.Origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM hosts WHERE id=%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("room creation did not wait for host lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false,revision=revision+1 WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != 404 {
			t.Fatalf("revoked permission accepted: %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("room request did not finish")
	}
}
