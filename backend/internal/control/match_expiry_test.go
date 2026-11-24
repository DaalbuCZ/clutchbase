package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestFailedLoadBlocksFurtherLoadsAndExpiryRequiresCapableAgent(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_match_setup,supports_map_observation,supports_match_setup_expiry) VALUES('machine','Machine','localhost',$1,'managed',now(),true,true,true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,access_status,active_preset,active_map,can_load_match,last_seen,match_phase) VALUES('server','Server','localhost',27015,'','machine','running','running','managed','verified','match','de_mirage',true,now(),'idle');
		INSERT INTO match_rooms(id,host_id,title,map,status,load_status) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Failed','de_mirage','closed','failed');
		INSERT INTO match_rooms(id,host_id,title,map) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','server','New','de_mirage');
		INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','76561198154367261','ct',true),('BCDEFGHIJKLMNOPQRSTUVWXY23','76561198154367262','t',true)`); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token string, body any, want int, out any) {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(res.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	rooms, err := a.matches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, room := range rooms {
		if room.Status == "open" {
			found = true
			if room.CanLoad || !room.LoadBlockedByFailure {
				t.Fatalf("new room did not show failed-load block: %+v", room)
			}
		}
	}
	if !found {
		t.Fatal("new room missing from snapshot")
	}
	path := "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/load"
	request("POST", path, a.AdminToken, nil, http.StatusConflict, nil)
	// Simulate a future safe recovery by clearing the failed test fixture.
	if _, err = a.DB.Exec(ctx, "UPDATE match_rooms SET load_status='' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'"); err != nil {
		t.Fatal(err)
	}
	request("POST", path, a.AdminToken, nil, http.StatusAccepted, nil)
	var enforced bool
	if err = a.DB.QueryRow(ctx, "SELECT load_expiry_enforced FROM match_rooms WHERE id='BCDEFGHIJKLMNOPQRSTUVWXY23'").Scan(&enforced); err != nil || !enforced {
		t.Fatalf("capability was not recorded: %v %v", enforced, err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMatchControl: true, SupportsMapObservation: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", ActiveMap: "de_mirage", MatchPhase: "idle", CanLoadMatch: true}}}}
	var desired protocol.MachineDesired
	request("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	if len(desired.Servers) != 1 || desired.Servers[0].MatchSetup != nil {
		t.Fatalf("expiry request sent to older agent: %+v", desired)
	}
	report.SupportsMatchSetupExpiry = true
	request("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	if len(desired.Servers) != 1 || desired.Servers[0].MatchSetup == nil || desired.Servers[0].MatchSetup.RequestedAt <= 0 {
		t.Fatalf("expiry request missing for updated agent: %+v", desired)
	}
	var supported bool
	if err = a.DB.QueryRow(ctx, "SELECT supports_match_setup_expiry FROM machines WHERE id='machine'").Scan(&supported); err != nil || !supported {
		t.Fatalf("capability was not persisted: %v %v", supported, err)
	}
	if strings.TrimSpace(desired.Servers[0].MatchSetup.ID) == "" {
		t.Fatal("missing setup ID")
	}
}
