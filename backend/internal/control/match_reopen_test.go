package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"clutchbase/internal/protocol"
)

func TestReopenRoomPermissionsAndConcurrency(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const owner = "76561198154367261"
	const other = "76561198154367262"
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1),($2); INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation,supports_whitelist) VALUES('machine','Machine','localhost','agent','managed',now(),true,true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,whitelist,whitelist_hash,active_preset,access_status,active_map,can_load_match,allow_player_rooms,match_phase) VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),ARRAY[$1,$2]::text[],$3,'match','verified','de_mirage',true,true,'idle'); INSERT INTO match_rooms(id,host_id,title,map,players_per_side,status,closed_at,created_by_steam_id) VALUES('room1','host','First','de_nuke',1,'closed',now(),$1),('room2','host','Second','de_nuke',1,'closed',now(),$1); INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('room1',$1,'ct',true),('room1',$2,'t',true),('room2',$1,'ct',true); INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($4,$1,now()+interval '1 hour'),($5,$2,now()+interval '1 hour')`, owner, other, protocol.WhitelistHash([]string{owner, other}), digest(strings.Repeat("a", 52)), digest(strings.Repeat("b", 52))); err != nil { t.Fatal(err) }
	call := func(room, user, body string) int {
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/reopen", strings.NewReader(body))
		if user != "" { req.AddCookie(&http.Cookie{Name: playerCookie, Value: strings.Repeat(user, 52)}) }
		req.Header.Set("Origin", a.Origin)
		res := httptest.NewRecorder(); a.Handler().ServeHTTP(res, req); return res.Code
	}
	if got := call("room1", "", `{"settingsRevision":1}`); got != 401 { t.Fatalf("anonymous: %d", got) }
	if got := call("room1", "b", `{"settingsRevision":1}`); got != 404 { t.Fatalf("non-owner: %d", got) }
	if got := call("room1", "a", `{"settingsRevision":0}`); got != 409 { t.Fatalf("stale: %d", got) }
	state, err := a.workspace(ctx); if err != nil { t.Fatal(err) }
	rooms, err := a.playerMatches(ctx, owner); if err != nil { t.Fatal(err) }
	view := playerWorkspace{Matches: rooms}; a.projectPlayerRoomActions(&view, state, owner)
	if len(view.Matches) != 2 || !view.Matches[0].Actions.Reopen || !view.Matches[1].Actions.Reopen { t.Fatalf("missing reopen actions: %+v", view.Matches) }
	results := make(chan int, 2); var wg sync.WaitGroup
	for _, room := range []string{"room1", "room2"} { wg.Add(1); go func(id string) { defer wg.Done(); results <- call(id, "a", `{"settingsRevision":1}`) }(room) }
	wg.Wait(); close(results); counts := map[int]int{}; for code := range results { counts[code]++ }
	if counts[204] != 1 || counts[409] != 1 { t.Fatalf("concurrent reopen results: %v", counts) }
	var open, ready int; var rev int64
	if err := a.DB.QueryRow(ctx, `SELECT count(*),max(settings_revision),(SELECT count(*) FROM match_room_players p JOIN match_rooms m ON m.id=p.match_id WHERE m.status='open' AND p.ready) FROM match_rooms WHERE host_id='host' AND status='open'`).Scan(&open, &rev, &ready); err != nil || open != 1 || rev != 2 || ready != 0 { t.Fatalf("open=%d revision=%d ready=%d: %v", open, rev, ready, err) }
}
