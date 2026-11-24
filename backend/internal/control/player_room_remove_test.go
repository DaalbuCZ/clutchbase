package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayerRoomCreatorRemovesParticipant(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const owner = "76561198154367261"
	const other = "76561198154367262"
	tokens := map[string]string{owner: strings.Repeat("a", 52), other: strings.Repeat("b", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES('76561198154367261','Owner'),('76561198154367262','Friend');
		INSERT INTO hosts(id,name,address,port,token_hash,whitelist,allow_player_rooms) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261','76561198154367262']::text[],true);
		INSERT INTO match_rooms(id,host_id,title,map,players_per_side,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',2,'76561198154367261');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',true);
		INSERT INTO match_map_votes(match_id,steam_id,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','de_nuke')`); err != nil {
		t.Fatal(err)
	}
	for id, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), id); err != nil {
			t.Fatal(err)
		}
	}
	state := func(id string) playerMatch {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/player/state", nil)
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		var view playerWorkspace
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil || len(view.Matches) != 1 {
			t.Fatalf("state: %d %s", w.Code, w.Body.String())
		}
		return view.Matches[0]
	}
	call := func(id, origin string, m playerMatch, target string) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"settingsRevision": m.SettingsRevision, "rosterFingerprint": m.RosterFingerprint, "removalToken": target})
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/remove-player", strings.NewReader(string(body)))
		if id != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	m := state(owner)
	target := ""
	for _, p := range m.Participants {
		if p.SteamID != "" {
			t.Fatal("SteamID exposed")
		}
		if p.GameName == "Friend" {
			target = p.RemovalToken
		}
		if p.GameName == "Owner" && p.RemovalToken != "" {
			t.Fatal("owner can remove themselves")
		}
	}
	if target == "" || m.RosterFingerprint == "" {
		t.Fatal("missing creator removal action")
	}
	for _, p := range state(other).Participants {
		if p.RemovalToken != "" {
			t.Fatal("noncreator received removal token")
		}
	}
	if got := call("", a.Origin, m, target); got != 401 {
		t.Fatalf("anonymous %d", got)
	}
	if got := call(owner, "https://attacker.invalid", m, target); got != 403 {
		t.Fatalf("origin %d", got)
	}
	if got := call(other, a.Origin, m, target); got != 404 {
		t.Fatalf("noncreator %d", got)
	}
	if got := call(owner, a.Origin, m, a.participantRemovalToken(room, owner)); got != 409 {
		t.Fatalf("self removal %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, a.Origin, m, target); got != 404 {
		t.Fatalf("revoked %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=true WHERE id='host'; UPDATE match_rooms SET load_status='pending' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'`); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, a.Origin, m, target); got != 404 {
		t.Fatalf("loaded %d", got)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET load_status='' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_room_players SET ready=false WHERE match_id=$1 AND steam_id=$2`, room, other); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, a.Origin, m, target); got != 409 {
		t.Fatalf("stale roster %d", got)
	}
	m = state(owner)
	if got := call(owner, a.Origin, m, target); got != 204 {
		t.Fatalf("remove %d", got)
	}
	m = state(owner)
	if m.Players != 1 || m.Ready != 0 {
		t.Fatalf("remaining roster %+v", m)
	}
	var allowed bool
	var votes int
	if err := a.DB.QueryRow(ctx, `SELECT $1=ANY(whitelist),(SELECT count(*) FROM match_map_votes WHERE match_id=$2) FROM hosts WHERE id='host'`, other, room).Scan(&allowed, &votes); err != nil || !allowed || votes != 0 {
		t.Fatalf("access/votes: %v %d %v", allowed, votes, err)
	}
}
