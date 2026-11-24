package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestMatchRoomPlayerFlow(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	steam := "76561198154367261"
	if _, err := a.DB.Exec(ctx, "INSERT INTO players(steam_id) VALUES($1)", steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, "INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_presets,supports_whitelist) VALUES('machine','Host','127.0.0.1','hash','managed',now(),true,true)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,map,desired_state,observed_state,active_map,driver,last_seen,access_status,whitelist,preset,applied_preset,active_preset,machine_id,can_load_match,match_phase)
		VALUES('match-host','Friends','127.0.0.1',27115,'hash','de_mirage','running','running','de_mirage','managed',now(),'verified',ARRAY[$1]::text[],'match','match','match','machine',true,'idle')`, steam); err != nil {
		t.Fatal(err)
	}
	playerToken := strings.Repeat("p", 52)
	if _, err := a.DB.Exec(ctx, "INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", digest(playerToken), steam); err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body any, admin bool, player bool, origin string) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		if admin {
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		}
		if player {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: playerToken})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	assertOpenMatch := func(want bool) {
		t.Helper()
		state, err := a.workspace(ctx)
		if err != nil || len(state.Hosts) != 1 || state.Hosts[0].Actions.OpenMatch != want {
			t.Fatalf("open match action: want %v, got %+v, error %v", want, state.Hosts, err)
		}
	}
	assertOpenMatch(false)
	profile := call("PATCH", "/api/player/profile", map[string]string{"gameName": "Friday Player"}, false, true, a.Origin)
	if profile.Code != 200 || !strings.Contains(profile.Body.String(), "Friday Player") {
		t.Fatal("player could not save an in-game name", profile.Code, profile.Body.String())
	}
	if response := call("PATCH", "/api/player/profile", map[string]string{"gameName": "cross-site"}, false, true, "https://attacker.example"); response.Code != 403 {
		t.Fatal("cross-origin profile update accepted", response.Code)
	}
	if response := call("POST", "/api/matches", map[string]any{"hostId": "match-host", "title": "Invalid map", "map": "de_nuke;quit"}, true, false, ""); response.Code != 400 {
		t.Fatal("unsupported match map accepted", response.Code, response.Body.String())
	}
	if response := call("POST", "/api/matches", map[string]any{"hostId": "match-host", "title": "Old agent"}, true, false, ""); response.Code != 409 {
		t.Fatal("agent without live map observation could create a room", response.Code, response.Body.String())
	}
	if _, err := a.DB.Exec(ctx, "UPDATE machines SET supports_map_observation=true WHERE id='machine'"); err != nil {
		t.Fatal(err)
	}
	assertOpenMatch(true)
	created := call("POST", "/api/matches", map[string]any{"hostId": "match-host", "title": "Friday friends", "playersPerSide": 1, "manualReady": true}, true, false, "")
	if created.Code != 201 {
		t.Fatalf("create match: %d %s", created.Code, created.Body.String())
	}
	assertOpenMatch(false)
	var match struct {
		ID             string `json:"id"`
		PlayersPerSide int    `json:"playersPerSide"`
		Map            string `json:"map"`
		ManualReady    bool   `json:"manualReady"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &match); err != nil || match.ID == "" || match.PlayersPerSide != 1 || match.Map != "de_mirage" || !match.ManualReady {
		t.Fatal("missing match id", err)
	}
	assertBlocker := func(code string) {
		t.Helper()
		adminRooms, err := a.matches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		adminFound := false
		for _, room := range adminRooms {
			for _, blocker := range room.LoadBlockers {
				if room.ID == match.ID && blocker == code {
					adminFound = true
				}
			}
		}
		playerRooms, err := a.playerMatches(ctx, steam)
		if err != nil {
			t.Fatal(err)
		}
		playerFound := false
		for _, room := range playerRooms {
			for _, blocker := range room.LoadBlockers {
				if room.ID == match.ID && blocker == code {
					playerFound = true
				}
			}
		}
		if !adminFound || !playerFound {
			t.Fatalf("load blocker %q absent from snapshots: admin=%v player=%v", code, adminFound, playerFound)
		}
	}
	assertBlocker("players_not_ready")
	if _, err := a.DB.Exec(ctx, "UPDATE match_rooms SET map='workshop/123456/test_map' WHERE id=$1", match.ID); err != nil {
		t.Fatal(err)
	}
	assertBlocker("workshop_unsupported")
	if _, err := a.DB.Exec(ctx, "UPDATE match_rooms SET map='de_mirage' WHERE id=$1", match.ID); err != nil {
		t.Fatal(err)
	}
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", nil, false, true, "https://attacker.example"); response.Code != 403 {
		t.Fatal("cross-origin player join accepted", response.Code)
	}
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", map[string]string{"side": "ct"}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("player could not join", response.Code, response.Body.String())
	}
	otherSteam, otherToken := "76561198154367262", strings.Repeat("q", 52)
	if _, err := a.DB.Exec(ctx, "INSERT INTO players(steam_id) VALUES($1)", otherSteam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET whitelist=ARRAY[$1,$2]::text[] WHERE id='match-host'", steam, otherSteam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, "INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", digest(otherToken), otherSteam); err != nil {
		t.Fatal(err)
	}
	playerToken = otherToken
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", map[string]string{"side": "ct"}, false, true, a.Origin); response.Code != 409 {
		t.Fatal("full side accepted another player", response.Code, response.Body.String())
	}
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", map[string]string{"side": "t"}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("available side rejected a player", response.Code, response.Body.String())
	}
	if response := call("PATCH", "/api/player/profile", map[string]string{"gameName": "T Side Player"}, false, true, a.Origin); response.Code != 200 {
		t.Fatal("second player could not save an in-game name", response.Code)
	}
	participants, err := a.playerMatches(ctx, steam)
	if err != nil || len(participants) != 1 || len(participants[0].Participants) != 2 || participants[0].Participants[0].GameName != "Friday Player" || participants[0].Participants[1].GameName != "T Side Player" {
		t.Fatalf("room roster did not show player-selected names: %+v %v", participants, err)
	}
	if response := call("POST", "/api/player/matches/"+match.ID+"/leave", nil, false, true, a.Origin); response.Code != 204 {
		t.Fatal("player could not leave room to free a slot", response.Code)
	}
	playerToken = strings.Repeat("p", 52)
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/side", map[string]string{"side": "t"}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("player could not choose a side", response.Code, response.Body.String())
	}
	ready := true
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": ready}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("player could not mark ready", response.Code, response.Body.String())
	}
	view, err := a.playerMatches(ctx, steam)
	if err != nil || len(view) != 1 || !view[0].Joined || !view[0].MyReady || view[0].Ready != 1 || view[0].MySide != "t" || view[0].TPlayers != 1 || len(view[0].Participants) != 1 || view[0].Participants[0].GameName != "Friday Player" {
		t.Fatalf("canonical player room state: %+v %v", view, err)
	}
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/side", map[string]string{"side": "ct"}, false, true, a.Origin); response.Code != 409 {
		t.Fatal("ready player changed sides", response.Code)
	}
	if response := call("PATCH", "/api/matches/"+match.ID, map[string]string{"status": "closed"}, true, false, ""); response.Code != 204 {
		t.Fatal("manager could not close match", response.Code, response.Body.String())
	}
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": false}, false, true, a.Origin); response.Code != 409 {
		t.Fatal("closed match accepted ready change", response.Code)
	}
	if response := call("POST", "/api/player/matches/"+match.ID+"/leave", nil, false, true, a.Origin); response.Code != 409 {
		t.Fatal("closed room accepted a roster change", response.Code)
	}
	view, err = a.playerMatches(ctx, steam)
	if err != nil || len(view) != 1 || !view[0].Joined || view[0].Actions.Leave || len(view[0].Participants) != 1 || !view[0].MyReady {
		t.Fatalf("closed room roster changed or exposed a leave action: %+v %v", view, err)
	}
	created = call("POST", "/api/matches", map[string]any{"hostId": "match-host", "title": "Ready MatchZy setup", "playersPerSide": 1, "map": "de_nuke"}, true, false, "")
	if created.Code != 201 {
		t.Fatal("create MatchZy room", created.Code, created.Body.String())
	}
	if err = json.Unmarshal(created.Body.Bytes(), &match); err != nil || match.Map != "de_nuke" {
		t.Fatal(err)
	}
	playerToken = strings.Repeat("p", 52)
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", map[string]string{"side": "ct"}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("join CT", response.Code, response.Body.String())
	}
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": true}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("ready CT", response.Code, response.Body.String())
	}
	playerToken = otherToken
	if response := call("POST", "/api/player/matches/"+match.ID+"/join", map[string]string{"side": "t"}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("join T", response.Code, response.Body.String())
	}
	ctView, err := a.playerMatches(ctx, steam)
	if err != nil || len(ctView) == 0 {
		t.Fatalf("could not read CT room: %+v %v", ctView, err)
	}
	foundCT := false
	for _, room := range ctView {
		if room.ID == match.ID {
			foundCT = true
			if room.MyReady {
				t.Fatalf("new player did not clear previous readiness: %+v", room)
			}
		}
	}
	if !foundCT {
		t.Fatalf("new player did not clear previous readiness: %+v %v", ctView, err)
	}
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": true}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("ready T", response.Code, response.Body.String())
	}
	playerToken = strings.Repeat("p", 52)
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": true}, false, true, a.Origin); response.Code != 204 {
		t.Fatal("ready CT after roster changed", response.Code, response.Body.String())
	}
	playerToken = otherToken
	if _, err = a.DB.Exec(ctx, "UPDATE machines SET supports_map_observation=false WHERE id='machine'"); err != nil {
		t.Fatal(err)
	}
	assertBlocker("reporter_unsupported")
	if response := call("POST", "/api/matches/"+match.ID+"/load", nil, true, false, ""); response.Code != http.StatusConflict {
		t.Fatal("room loaded with old agent", response.Code, response.Body.String())
	}
	if _, err = a.DB.Exec(ctx, "UPDATE machines SET supports_map_observation=true WHERE id='machine'"); err != nil {
		t.Fatal(err)
	}
	if response := call("POST", "/api/matches/"+match.ID+"/load", nil, true, false, ""); response.Code != http.StatusAccepted {
		t.Fatal("load MatchZy", response.Code, response.Body.String())
	}
	if response := call("PATCH", "/api/player/profile", map[string]string{"gameName": "Renamed after request"}, false, true, a.Origin); response.Code != http.StatusOK {
		t.Fatal("profile change while loading failed", response.Code, response.Body.String())
	}
	var pending struct {
		LoadStatus string `json:"loadStatus"`
		CanLoad    bool   `json:"canLoad"`
	}
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) < 1 {
		t.Fatal("list loaded room", err)
	}
	for _, room := range rooms {
		if room.ID == match.ID {
			if room.CanClose {
				t.Fatal("pending room advertised close")
			}
			pending.LoadStatus = room.LoadStatus
			pending.CanLoad = room.CanLoad
			if len(room.Participants) != 2 || room.Participants[1].GameName != "T Side Player" {
				t.Fatalf("management room lost frozen name: %+v", room.Participants)
			}
		}
	}
	playerRooms, err := a.playerMatches(ctx, otherSteam)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range playerRooms {
		if room.ID == match.ID && (len(room.Participants) != 2 || room.Participants[1].GameName != "T Side Player") {
			t.Fatalf("player room lost frozen name: %+v", room.Participants)
		}
	}
	if pending.LoadStatus != "pending" || pending.CanLoad {
		t.Fatalf("room did not canonicalize pending load: %+v", pending)
	}
	if response := call("PATCH", "/api/matches/"+match.ID, map[string]any{"status": "closed", "confirmLoaded": true}, true, false, ""); response.Code != http.StatusConflict {
		t.Fatal("pending setup allowed room close", response.Code)
	}
	if response := call("PATCH", "/api/player/matches/"+match.ID+"/ready", map[string]bool{"ready": false}, false, true, a.Origin); response.Code != 409 {
		t.Fatal("players could change a loaded roster", response.Code)
	}
	// The authenticated machine agent receives the immutable server-generated setup and acknowledges its command ID.
	if _, err = a.DB.Exec(ctx, "UPDATE machines SET token_hash=$1 WHERE id='machine'", digest("machine-secret")); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader([]byte(`{"driver":"managed","supportsRemoval":true,"supportsWhitelist":true,"supportsPresets":true,"supportsLiveMode":true,"supportsMatchSetup":true,"servers":[{"id":"match-host","driver":"managed","state":"running","activeMap":"de_mirage","accessStatus":"verified","activePreset":"match","canLiveMode":true,"canLoadMatch":true}]}`)))
	request.Header.Set("Authorization", "Bearer machine-secret")
	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("machine heartbeat request", response.Code, response.Body.String())
	}
	var desired protocol.MachineDesired
	if err = json.Unmarshal(response.Body.Bytes(), &desired); err != nil || len(desired.Servers) != 1 || desired.Servers[0].MatchSetup != nil {
		t.Fatal("pending setup delivered to an old agent", err, response.Body.String())
	}
	request = httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader([]byte(`{"driver":"managed","supportsRemoval":true,"supportsWhitelist":true,"supportsPresets":true,"supportsLiveMode":true,"supportsMatchSetup":true,"supportsMapObservation":true,"servers":[{"id":"match-host","driver":"managed","state":"running","activeMap":"de_mirage","accessStatus":"verified","activePreset":"match","canLiveMode":true,"canLoadMatch":true}]}`)))
	request.Header.Set("Authorization", "Bearer machine-secret")
	response = httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("updated agent heartbeat", response.Code, response.Body.String())
	}
	if err = json.Unmarshal(response.Body.Bytes(), &desired); err != nil || len(desired.Servers) != 1 || desired.Servers[0].MatchSetup == nil || desired.Servers[0].MatchSetup.Map != "de_nuke" || len(desired.Servers[0].MatchSetup.T) != 1 || desired.Servers[0].MatchSetup.T[0].GameName != "T Side Player" {
		t.Fatal("agent did not receive MatchZy setup", err, response.Body.String())
	}
	firstSetup := *desired.Servers[0].MatchSetup
	if firstSetup.RequestedAt == 0 {
		t.Fatal("setup did not carry its original request time")
	}
	commandID := firstSetup.ID
	if response := call("PATCH", "/api/player/profile", map[string]string{"gameName": "Changed again"}, false, true, a.Origin); response.Code != http.StatusOK {
		t.Fatal("second profile change failed", response.Code)
	}
	request = httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader([]byte(`{"driver":"managed","supportsRemoval":true,"supportsWhitelist":true,"supportsPresets":true,"supportsLiveMode":true,"supportsMatchSetup":true,"supportsMapObservation":true,"servers":[{"id":"match-host","driver":"managed","state":"running","activeMap":"de_mirage","accessStatus":"verified","activePreset":"match","canLiveMode":true,"canLoadMatch":true}]}`)))
	request.Header.Set("Authorization", "Bearer machine-secret")
	response = httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	var retried protocol.MachineDesired
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &retried) != nil || len(retried.Servers) != 1 || retried.Servers[0].MatchSetup == nil || !reflect.DeepEqual(firstSetup, *retried.Servers[0].MatchSetup) {
		t.Fatalf("request changed on replay: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader([]byte(fmt.Sprintf(`{"driver":"managed","supportsRemoval":true,"supportsWhitelist":true,"supportsPresets":true,"supportsLiveMode":true,"supportsMatchSetup":true,"supportsMapObservation":true,"servers":[{"id":"match-host","driver":"managed","state":"running","activeMap":"de_mirage","accessStatus":"verified","activePreset":"match","canLiveMode":true,"canLoadMatch":true,"matchSetupId":%q,"matchSetupStatus":"complete"}]}`, commandID))))
	request.Header.Set("Authorization", "Bearer machine-secret")
	response = httptest.NewRecorder()
	a.Handler().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("machine setup acknowledgement", response.Code, response.Body.String())
	}
	rooms, err = a.matches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range rooms {
		if room.ID == match.ID && (room.LoadStatus != "complete" || len(room.Participants) != 2 || room.Participants[1].GameName != "T Side Player") {
			t.Fatalf("room did not preserve acknowledged setup: %+v", room)
		}
		if room.ID == match.ID && !room.CanClose {
			t.Fatal("loaded room did not advertise close")
		}
	}
	if response := call("PATCH", "/api/matches/"+match.ID, map[string]any{"status": "closed"}, true, false, ""); response.Code != http.StatusConflict {
		t.Fatal("loaded room closed without acknowledgement", response.Code)
	}
	if response := call("PATCH", "/api/matches/"+match.ID, map[string]any{"status": "closed", "confirmLoaded": true}, true, false, ""); response.Code != http.StatusNoContent {
		t.Fatal("loaded room could not be closed after acknowledgement", response.Code, response.Body.String())
	}
}

func TestUpgradeInterruptsUnrecordedPendingMatchSetup(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,map) VALUES('host','Host','localhost',27015,'','de_mirage'); INSERT INTO match_rooms(id,host_id,title,map,load_status,load_command_id,load_requested_at) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage','pending','ABCDEFGHIJKLMNOPQRSTUVWX23',now()); ALTER TABLE match_rooms DROP COLUMN load_payload, DROP COLUMN load_retry_after; DELETE FROM schema_migrations WHERE version=18`); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	var payloadIsNull, waiting bool
	if err := a.DB.QueryRow(ctx, `SELECT load_status,load_payload IS NULL,load_retry_after>now() FROM match_rooms WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'`).Scan(&status, &payloadIsNull, &waiting); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || !payloadIsNull || !waiting {
		t.Fatalf("legacy pending command was not safely interrupted: %s %v %v", status, payloadIsNull, waiting)
	}
}

func TestInvalidSavedMatchSetupDoesNotBlockHeartbeat(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,map,machine_id,desired_state) VALUES('server','Server','localhost',27015,'','de_mirage','machine','running')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,load_status,load_command_id,load_requested_at,load_payload) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Room','de_mirage','pending','ABCDEFGHIJKLMNOPQRSTUVWX23',now(),'{}'::jsonb)`); err != nil {
		t.Fatal(err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMapObservation: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{State: "running", Driver: "managed", ActiveMap: "de_mirage", AccessStatus: "verified", ActivePreset: "match", CanLoadMatch: true}}}}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/machine-agent/machine/heartbeat", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer agent-token")
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, req)
	var desired protocol.MachineDesired
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &desired) != nil || len(desired.Servers) != 1 || desired.Servers[0].MatchSetup != nil {
		t.Fatalf("invalid setup blocked heartbeat: %d %s", res.Code, res.Body.String())
	}
	var status string
	if err := a.DB.QueryRow(ctx, "SELECT load_status FROM match_rooms WHERE host_id='server'").Scan(&status); err != nil || status != "failed" {
		t.Fatalf("invalid setup was not failed: %s %v", status, err)
	}
}
