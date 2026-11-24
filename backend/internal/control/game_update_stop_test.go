package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestStopAndUpdateWaitsForStoppedAgentAndPreservesStoppedState(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := "machine-secret"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,driver,supports_game_update) VALUES('machine','Machine','127.0.0.1',$1,now(),'managed',true)`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen) VALUES('server','Server','127.0.0.1',27015,'','machine','running','running','managed',now())`); err != nil {
		t.Fatal(err)
	}
	update := func(body string) int {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/hosts/server/game-update", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	state, err := a.workspace(ctx)
	if err != nil || len(state.Hosts) != 1 || !state.Hosts[0].Actions.StopAndUpdate {
		t.Fatalf("no stop-update action %+v %v", state.Hosts, err)
	}
	if _, err = a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Match','de_mirage')`); err != nil {
		t.Fatal(err)
	}
	state, err = a.workspace(ctx)
	if err != nil || state.Hosts[0].Actions.StopAndUpdate {
		t.Fatalf("open room allowed update %+v %v", state.Hosts, err)
	}
	if got := update(`{"revision":1,"stopFirst":true}`); got != 409 {
		t.Fatalf("open match accepted: %d", got)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed' WHERE host_id='server'`); err != nil {
		t.Fatal(err)
	}
	if got := update(`{"revision":2,"stopFirst":true}`); got != 409 {
		t.Fatalf("stale revision accepted: %d", got)
	}
	if got := update(`{"revision":1,"stopFirst":true}`); got != 202 {
		t.Fatalf("request rejected: %d", got)
	}
	state, err = a.workspace(ctx)
	if err != nil || state.Hosts[0].DesiredState != "stopped" || state.Hosts[0].ObservedState != "running" || state.Hosts[0].GameUpdateStatus != "pending" || state.Hosts[0].Actions.Start || state.Hosts[0].Actions.StopAndUpdate {
		t.Fatalf("request changed observed state or actions: %+v %v", state.Hosts, err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsGameUpdate: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running"}}}}
	heartbeat := func() protocol.MachineDesired {
		t.Helper()
		body, _ := json.Marshal(report)
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("heartbeat %d: %s", w.Code, w.Body.String())
		}
		var result protocol.MachineDesired
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	desired := heartbeat()
	if len(desired.Servers) != 1 || desired.Servers[0].State != "stopped" || desired.Servers[0].GameUpdate != nil {
		t.Fatalf("update delivered before stop: %+v", desired)
	}
	report.Servers[0].State = "stopped"
	desired = heartbeat()
	if desired.Servers[0].GameUpdate == nil {
		t.Fatalf("stopped agent got no update: %+v", desired)
	}
	command := desired.Servers[0].GameUpdate.ID
	report.Servers[0].GameUpdateID = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	report.Servers[0].GameUpdateStatus = "complete"
	desired = heartbeat()
	if desired.Servers[0].GameUpdate == nil {
		t.Fatal("unrelated acknowledgment accepted")
	}
	report.Servers[0].GameUpdateID = command
	report.Servers[0].GameUpdateStatus = "failed"
	report.Servers[0].GameUpdateError = "SteamCMD failed"
	heartbeat()
	state, err = a.workspace(ctx)
	if err != nil || state.Hosts[0].DesiredState != "stopped" || state.Hosts[0].ObservedState != "stopped" || state.Hosts[0].GameUpdateStatus != "failed" || !state.Hosts[0].Actions.UpdateGame {
		t.Fatalf("failure did not leave stopped retryable state: %+v %v", state.Hosts, err)
	}
}
