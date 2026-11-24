package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchSettingsPreservePlayersAndRequireReadinessAgain(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const roomID = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,'');
		INSERT INTO match_rooms(id,host_id,title,map,players_per_side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',3);
		INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'),('76561198154367263');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES
		('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),
		('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','ct',true),
		('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367263','t',true)`); err != nil {
		t.Fatal(err)
	}
	request := func(token, title, mapName string, size int, revision any, want int) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"title": title, "map": mapName, "playersPerSide": size, "overtime": false, "settingsRevision": revision})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("PATCH", "/api/matches/"+roomID+"/settings", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("got %d, want %d: %s", res.Code, want, res.Body.String())
		}
	}
	assertRoom := func(title, mapName string, size, ready int, revision int64, editable bool) {
		t.Helper()
		rooms, err := a.matches(ctx)
		if err != nil || len(rooms) != 1 {
			t.Fatalf("room snapshot: %+v %v", rooms, err)
		}
		room := rooms[0]
		if room.Title != title || room.Map != mapName || room.PlayersPerSide != size || room.Players != 3 || room.Ready != ready || room.SettingsRevision != revision || room.CanEdit != editable {
			t.Fatalf("unexpected room: %+v", room)
		}
	}
	admin := a.AdminToken
	assertRoom("Friends", "de_mirage", 3, 3, 1, true)
	request("wrong", "Friends", "de_nuke", 2, 1, 401)
	request(admin, "Friends", "de_nuke;quit", 2, 1, 400)
	request(admin, "Friends", "de_mirage", 0, 1, 400)
	request(admin, "Friends", "de_mirage", 3, nil, 409)
	request(admin, "Friday friends", "de_mirage", 3, 1, 200)
	assertRoom("Friday friends", "de_mirage", 3, 3, 2, true)
	request(admin, "Friday friends", "de_nuke", 1, 2, 409)
	overtimeBody, _ := json.Marshal(map[string]any{"title": "Friday friends", "map": "de_mirage", "playersPerSide": 3, "overtime": true, "settingsRevision": 2})
	overtimeReq := httptest.NewRequest("PATCH", "/api/matches/"+roomID+"/settings", strings.NewReader(string(overtimeBody)))
	overtimeReq.Header.Set("Authorization", "Bearer "+admin)
	overtimeRes := httptest.NewRecorder()
	a.Handler().ServeHTTP(overtimeRes, overtimeReq)
	if overtimeRes.Code != 200 {
		t.Fatalf("enable overtime: %d %s", overtimeRes.Code, overtimeRes.Body.String())
	}
	var overtimeEnabled bool
	if err := a.DB.QueryRow(ctx, "SELECT overtime FROM match_rooms WHERE id=$1", roomID).Scan(&overtimeEnabled); err != nil || !overtimeEnabled {
		t.Fatalf("overtime was not persisted: %v %v", overtimeEnabled, err)
	}
	var result struct {
		ReadinessReset bool `json:"readinessReset"`
	}
	if err := json.Unmarshal(overtimeRes.Body.Bytes(), &result); err != nil || !result.ReadinessReset {
		t.Fatalf("overtime must report reset: %+v %v", result, err)
	}
	assertRoom("Friday friends", "de_mirage", 3, 0, 3, true)
	if _, err := a.DB.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1`, roomID); err != nil {
		t.Fatal(err)
	}
	request(admin, "Friday friends", "de_mirage", 3, 3, 200)
	assertRoom("Friday friends", "de_mirage", 3, 0, 4, true)
	if _, err := a.DB.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1`, roomID); err != nil {
		t.Fatal(err)
	}
	request(admin, "Friday friends", "de_mirage", 3, 4, 200)
	assertRoom("Friday friends", "de_mirage", 3, 3, 4, true)
	request(admin, "Friday friends", "de_nuke", 2, 4, 200)
	assertRoom("Friday friends", "de_nuke", 2, 0, 5, true)
	request(admin, "Stale editor", "de_train", 3, 4, 409)
	assertRoom("Friday friends", "de_nuke", 2, 0, 5, true)
	for _, status := range []string{"pending", "complete", "failed"} {
		if _, err := a.DB.Exec(ctx, "UPDATE match_rooms SET load_status=$2 WHERE id=$1", roomID, status); err != nil {
			t.Fatal(err)
		}
		request(admin, "Locked", "de_nuke", 2, 5, 409)
		assertRoom("Friday friends", "de_nuke", 2, 0, 5, false)
	}
	if _, err := a.DB.Exec(ctx, "UPDATE match_rooms SET load_status='',status='closed' WHERE id=$1", roomID); err != nil {
		t.Fatal(err)
	}
	request(admin, "Closed", "de_nuke", 2, 5, 409)
	assertRoom("Friday friends", "de_nuke", 2, 0, 5, false)
}

func TestOvertimeChangeUpdatesPlayerLiveReadiness(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	token := strings.Repeat("p", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261');
		INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261']::text[]);
		INSERT INTO match_rooms(id,host_id,title,map,players_per_side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',1);
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var before playerWorkspace
	readLive(t, c, &before)
	if len(before.Matches) != 1 || !before.Matches[0].MyReady || before.Matches[0].Overtime {
		t.Fatalf("unexpected initial state: %+v", before.Matches)
	}
	req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/settings", strings.NewReader(`{"title":"Friends","map":"de_mirage","playersPerSide":1,"overtime":true,"settingsRevision":1}`))
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings: %d %s", res.Code, res.Body.String())
	}
	var after playerWorkspace
	readLive(t, c, &after)
	if len(after.Matches) != 1 || after.Matches[0].MyReady || after.Matches[0].Ready != 0 || !after.Matches[0].Overtime {
		t.Fatalf("player did not receive changed rules and readiness: %+v", after.Matches)
	}
}
