package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManagerRemovesRoomPlayerWithoutRevokingAccess(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const first = "76561198154367261"
	const second = "76561198154367262"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,desired_state,observed_state,driver,last_seen,access_status,active_preset,whitelist)
		VALUES('host','Host','localhost',27015,'','running','running','managed',now(),'verified','match',ARRAY['76561198154367261','76561198154367262']);
		INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage');
		INSERT INTO players(steam_id,game_name) VALUES('76561198154367261','First'),('76561198154367262','Second');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES
		('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',true)`); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token string, body string, playerCookieValue string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if playerCookieValue != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: playerCookieValue})
			req.Header.Set("Origin", a.Origin)
		}
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		return res
	}
	path := "/api/matches/" + room + "/players/" + first
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || !rooms[0].CanRemovePlayer || len(rooms[0].Participants) != 2 {
		t.Fatalf("manager roster: %+v %v", rooms, err)
	}
	ids := map[string]bool{}
	for _, participant := range rooms[0].Participants {
		ids[participant.SteamID] = true
	}
	if !ids[first] || !ids[second] {
		t.Fatalf("manager roster lacks participant IDs: %+v", rooms[0].Participants)
	}
	playerRooms, err := a.playerMatches(ctx, second)
	if err != nil || len(playerRooms) != 1 {
		t.Fatalf("player roster: %+v %v", playerRooms, err)
	}
	encoded, err := json.Marshal(playerRooms)
	if err != nil || strings.Contains(string(encoded), "steamId") || strings.Contains(string(encoded), first) {
		t.Fatalf("player snapshot exposed room SteamIDs: %s %v", encoded, err)
	}
	if res := call("DELETE", path, "", "", ""); res.Code != http.StatusUnauthorized {
		t.Fatal("unauthorized removal", res.Code)
	}
	if res := call("DELETE", "/api/matches/"+room+"/players/invalid", a.AdminToken, "", ""); res.Code != http.StatusBadRequest {
		t.Fatal("invalid player accepted", res.Code)
	}
	if res := call("DELETE", path, a.AdminToken, "", ""); res.Code != http.StatusNoContent {
		t.Fatal("remove player", res.Code, res.Body.String())
	}
	rooms, err = a.matches(ctx)
	if err != nil || len(rooms) != 1 || rooms[0].Players != 1 || rooms[0].Ready != 0 || len(rooms[0].Participants) != 1 || rooms[0].Participants[0].SteamID != second {
		t.Fatalf("remaining roster: %+v %v", rooms, err)
	}
	if res := call("DELETE", path, a.AdminToken, "", ""); res.Code != http.StatusNotFound {
		t.Fatal("missing player removal", res.Code)
	}
	var allowed bool
	if err = a.DB.QueryRow(ctx, "SELECT $1=ANY(whitelist) FROM hosts WHERE id='host'", first).Scan(&allowed); err != nil || !allowed {
		t.Fatalf("server access changed: %v %v", allowed, err)
	}
	playerToken := strings.Repeat("p", 52)
	if _, err = a.DB.Exec(ctx, "INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", digest(playerToken), first); err != nil {
		t.Fatal(err)
	}
	if res := call("POST", "/api/player/matches/"+room+"/join", "", `{"side":"ct"}`, playerToken); res.Code != http.StatusNoContent {
		t.Fatal("removed player could not rejoin", res.Code, res.Body.String())
	}
	for _, status := range []string{"pending", "complete", "failed"} {
		if _, err = a.DB.Exec(ctx, "UPDATE match_rooms SET load_status=$2 WHERE id=$1", room, status); err != nil {
			t.Fatal(err)
		}
		if res := call("DELETE", path, a.AdminToken, "", ""); res.Code != http.StatusConflict {
			t.Fatal("locked roster accepted removal", status, res.Code)
		}
	}
}
