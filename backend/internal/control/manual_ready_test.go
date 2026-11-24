package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"clutchbase/internal/protocol"
)

func TestManualReadyCapabilityIsPersistedAndRequiredForLoad(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation)
		VALUES('machine','Machine','localhost',$1,'managed',now(),true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,access_status,active_preset,active_map,can_load_match,match_phase)
		VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),'verified','match','de_mirage',true,'idle')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,manual_ready,players_per_side) VALUES($1,'host','Manual ready','de_mirage',true,1)`, room); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES($1,'76561198154367261','ct',true),($1,'76561198154367262','t',true)`, room); err != nil {
		t.Fatal(err)
	}
	matches, err := a.matches(ctx)
	if err != nil || len(matches) != 1 || matches[0].CanLoad || !matches[0].ManualReady {
		t.Fatalf("manual-ready room should be blocked before capability advertisement: %+v, %v", matches, err)
	}
	callHeartbeat := func(supported bool) protocol.MachineDesired {
		t.Helper()
		report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMatchControl: true, SupportsMapObservation: true,
			Servers: []protocol.ServerReport{{ID: "host", Report: protocol.Report{State: "running", Driver: "managed", ActiveMap: "de_mirage", AccessStatus: "verified", ActivePreset: "match", MatchPhase: "idle", CanLoadMatch: true, SupportsManualReady: supported}}}}
		data, _ := json.Marshal(report)
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer agent-token")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("heartbeat: %d %s", w.Code, w.Body.String())
		}
		var desired protocol.MachineDesired
		if err := json.Unmarshal(w.Body.Bytes(), &desired); err != nil {
			t.Fatal(err)
		}
		return desired
	}
	callHeartbeat(true)
	var supported bool
	if err := a.DB.QueryRow(ctx, `SELECT supports_manual_ready FROM hosts WHERE id='host'`).Scan(&supported); err != nil || !supported {
		t.Fatalf("manual-ready capability was not persisted: %v %v", supported, err)
	}
	matches, err = a.matches(ctx)
	if err != nil || len(matches) != 1 || !matches[0].CanLoad {
		t.Fatalf("manual-ready room should load after capability advertisement: %+v, %v", matches, err)
	}
	patch := httptest.NewRequest("PATCH", "/api/matches/"+room+"/settings", bytes.NewBufferString(`{"title":"Manual ready","map":"de_nuke","playersPerSide":1,"overtime":false,"settingsRevision":1}`))
	patch.Header.Set("Authorization", "Bearer "+a.AdminToken)
	patchResponse := httptest.NewRecorder()
	a.Handler().ServeHTTP(patchResponse, patch)
	if patchResponse.Code != 200 {
		t.Fatalf("legacy settings update: %d %s", patchResponse.Code, patchResponse.Body.String())
	}
	var manualReady bool
	if err := a.DB.QueryRow(ctx, `SELECT manual_ready FROM match_rooms WHERE id=$1`, room).Scan(&manualReady); err != nil || !manualReady {
		t.Fatalf("legacy settings update cleared manual-ready: %v %v", manualReady, err)
	}
	callHeartbeat(false) // Older agents omit the capability and must fail closed.
	matches, err = a.matches(ctx)
	if err != nil || len(matches) != 1 || matches[0].CanLoad {
		t.Fatalf("room load was not blocked after capability disappeared: %+v, %v", matches, err)
	}

}
