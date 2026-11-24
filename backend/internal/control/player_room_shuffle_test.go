package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayerRoomShufflePermissionsAndRoster(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const owner = "76561198154367261"
	const other = "76561198154367262"
	tokens := map[string]string{owner: strings.Repeat("a", 52), other: strings.Repeat("b", 52)}
	_, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262');
		INSERT INTO hosts(id,name,address,port,token_hash,whitelist,allow_player_rooms) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261','76561198154367262']::text[],true);
		INSERT INTO match_rooms(id,host_id,title,map,players_per_side,created_by_steam_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',2,'76561198154367261');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','ct',true)`)
	if err != nil {
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
		if w.Code != 200 {
			t.Fatalf("state: %d %s", w.Code, w.Body.String())
		}
		var view playerWorkspace
		if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || len(view.Matches) != 1 {
			t.Fatalf("state: %+v %v", view, err)
		}
		for _, participant := range view.Matches[0].Participants {
			if participant.SteamID != "" {
				t.Fatal("public roster exposes a SteamID")
			}
		}
		return view.Matches[0]
	}
	call := func(id, origin string, m playerMatch) int {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"settingsRevision": m.SettingsRevision, "rosterFingerprint": m.RosterFingerprint})
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/shuffle", strings.NewReader(string(body)))
		if id != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[id]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	m := state(owner)
	if !m.Actions.Shuffle || len(m.RosterFingerprint) != 64 {
		t.Fatalf("missing creator action: %+v", m)
	}
	if state(other).Actions.Shuffle {
		t.Fatal("noncreator can shuffle")
	}
	if got := call("", a.Origin, m); got != 401 {
		t.Fatalf("anonymous %d", got)
	}
	if got := call(owner, "https://attacker.invalid", m); got != 403 {
		t.Fatalf("origin %d", got)
	}
	if got := call(other, a.Origin, m); got != 404 {
		t.Fatalf("noncreator %d", got)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_room_players SET ready=false WHERE match_id=$1 AND steam_id=$2`, room, other); err != nil {
		t.Fatal(err)
	}
	if got := call(owner, a.Origin, m); got != 409 {
		t.Fatalf("stale roster %d", got)
	}
	m = state(owner)
	if got := call(owner, a.Origin, m); got != 204 {
		t.Fatalf("shuffle %d", got)
	}
	m = state(owner)
	if m.Ready != 0 || m.CTPlayers != 1 || m.TPlayers != 1 {
		t.Fatalf("invalid result: %+v", m)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE hosts SET allow_player_rooms=false WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if state(owner).Actions.Shuffle {
		t.Fatal("revoked shuffle action exposed")
	}
	if got := call(owner, a.Origin, m); got != 404 {
		t.Fatalf("revoked %d", got)
	}
}
