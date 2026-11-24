package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"clutchbase/internal/protocol"
)

func TestCancelFailedMatchRequiresFencedAcknowledgement(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_match_setup,supports_map_observation,supports_match_setup_expiry,supports_match_control,supports_match_cancel) VALUES('machine','Machine','localhost',$1,'managed',now(),true,true,true,true,true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,access_status,active_preset,active_map,match_phase,can_load_match,supports_match_fence,last_seen,revision) VALUES('server','Server','localhost',27015,'','machine','running','running','managed','verified','match','de_mirage','idle',true,true,now(),1);
		INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Room','de_mirage');
		INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',true)`); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token string, body any, want int, out any) {
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
	call("POST", "/api/matches/"+room+"/load", a.AdminToken, nil, http.StatusAccepted, nil)
	var setupID string
	var fenced, expiry bool
	if err := a.DB.QueryRow(ctx, "SELECT load_command_id,load_fence_required,load_expiry_enforced FROM match_rooms WHERE id=$1", room).Scan(&setupID, &fenced, &expiry); err != nil || !fenced || !expiry {
		t.Fatalf("missing durable fence requirements: %s %v %v %v", setupID, fenced, expiry, err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMatchSetupExpiry: true, SupportsMatchControl: true, SupportsMapObservation: true, SupportsMatchCancel: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", ActiveMap: "de_mirage", MatchPhase: "idle", CanLoadMatch: true, SupportsMatchFence: true, MatchSetupID: setupID, MatchSetupStatus: "failed", MatchSetupError: "Not issued"}}}}
	var desired protocol.MachineDesired
	heartbeat := func() {
		t.Helper()
		call("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	}
	heartbeat()
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || !rooms[0].CanCancelLoad || !rooms[0].LoadBlockedByFailure {
		t.Fatalf("cancellation not offered: %+v %v", rooms, err)
	}
	path := "/api/matches/" + room + "/cancel-load"
	call("POST", path, "invalid", map[string]any{"revision": 1}, http.StatusUnauthorized, nil)
	call("POST", path, a.AdminToken, map[string]any{"revision": 0}, http.StatusConflict, nil)
	call("POST", path, a.AdminToken, map[string]any{"revision": 1}, http.StatusAccepted, nil)
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CancelStatus != "pending" || rooms[0].LoadStatus != "failed" {
		t.Fatalf("cancellation completed optimistically: %+v %v", rooms, err)
	}
	heartbeat()
	if len(desired.Servers) != 1 || desired.Servers[0].MatchCancel == nil || desired.Servers[0].MatchCancel.SetupID != setupID {
		t.Fatalf("missing cancellation: %+v", desired)
	}
	command := desired.Servers[0].MatchCancel
	report.Servers[0].MatchCancelID = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	report.Servers[0].MatchCancelSetupID = setupID
	report.Servers[0].MatchCancelStatus = "complete"
	heartbeat()
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CancelStatus != "pending" {
		t.Fatalf("unrelated acknowledgement accepted: %+v %v", rooms, err)
	}
	report.Servers[0].MatchCancelID = command.ID
	report.Servers[0].MatchCancelSetupID = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	heartbeat()
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CancelStatus != "pending" {
		t.Fatalf("acknowledgement for another setup accepted: %+v %v", rooms, err)
	}
	report.Servers[0].MatchCancelSetupID = setupID
	report.Servers[0].CanLoadMatch = false
	heartbeat()
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CancelStatus != "pending" {
		t.Fatalf("unavailable runtime accepted: %+v %v", rooms, err)
	}
	report.Servers[0].CanLoadMatch = true
	heartbeat()
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].LoadStatus != "" || rooms[0].CancelStatus != "complete" || rooms[0].LoadBlockedByFailure || rooms[0].Ready != 0 || rooms[0].Players != 2 || !rooms[0].CanEdit {
		t.Fatalf("room was not safely reopened: %+v %v", rooms, err)
	}
	heartbeat()
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CancelStatus != "complete" || rooms[0].Ready != 0 {
		t.Fatalf("replay changed recovered room: %+v %v", rooms, err)
	}
}

func TestLegacyFailedLoadCannotBeCancelled(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,''); INSERT INTO match_rooms(id,host_id,title,map,load_status,load_command_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Legacy','de_mirage','failed','BCDEFGHIJKLMNOPQRSTUVWXY23')`); err != nil {
		t.Fatal(err)
	}
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || rooms[0].CanCancelLoad {
		t.Fatalf("legacy setup was offered cancellation: %+v %v", rooms, err)
	}
}

func TestUncertainCancellationKeepsLoadBlocked(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const cancel = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state) VALUES('server','Server','localhost',27015,'','machine','running')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,load_status,load_command_id,load_payload,load_expiry_enforced,load_fence_required,cancel_id,cancel_status)
		VALUES($1,'server','Room','de_mirage','failed',$1,jsonb_build_object('id',$1::text,'fenceRequired',true),true,true,$2,'pending')`, setup, cancel); err != nil {
		t.Fatal(err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMatchControl: true, SupportsMatchCancel: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", ActiveMap: "de_mirage", MatchPhase: "idle", CanLoadMatch: true, SupportsMatchFence: true, MatchCancelID: cancel, MatchCancelSetupID: setup, MatchCancelStatus: "failed", MatchCancelError: "MatchZy may already have received the setup"}}}}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/machine-agent/machine/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer agent-token")
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("heartbeat %d: %s", res.Code, res.Body.String())
	}
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || rooms[0].LoadStatus != "failed" || rooms[0].CancelStatus != "failed" || rooms[0].CancelError == "" || !rooms[0].LoadBlockedByFailure {
		t.Fatalf("uncertain cancellation cleared the block: %+v %v", rooms, err)
	}
}
