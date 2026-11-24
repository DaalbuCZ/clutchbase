package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestCancelQueuedGameUpdateBeforeAndAfterDelivery(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := "agent-secret"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,driver,supports_game_update) VALUES('machine','Machine','localhost',$1,now(),'managed',true)`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen) VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now())`); err != nil {
		t.Fatal(err)
	}
	call := func(path, token, body string) int {
		t.Helper()
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := call("/api/hosts/host/game-update", a.AdminToken, `{"revision":1,"stopFirst":true}`); code != 202 {
		t.Fatalf("request %d", code)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET last_seen=now()-interval '1 hour' WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	state, err := a.workspace(ctx)
	if err != nil || !state.Hosts[0].Actions.CancelGameUpdate {
		t.Fatalf("offline queued update not cancellable %+v %v", state.Hosts, err)
	}
	if code := call("/api/hosts/host/game-update/cancel", "wrong", `{"revision":2}`); code != 401 {
		t.Fatalf("unauthorized %d", code)
	}
	if code := call("/api/hosts/host/game-update/cancel", a.AdminToken, `{"revision":1}`); code != 409 {
		t.Fatalf("stale %d", code)
	}
	if code := call("/api/hosts/host/game-update/cancel", a.AdminToken, `{"revision":2}`); code != 204 {
		t.Fatalf("cancel %d", code)
	}
	state, err = a.workspace(ctx)
	if err != nil || state.Hosts[0].GameUpdateStatus != "cancelled" || state.Hosts[0].DesiredState != "stopped" || state.Hosts[0].Actions.CancelGameUpdate {
		t.Fatalf("bad cancel state %+v %v", state.Hosts, err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsGameUpdate: true, Servers: []protocol.ServerReport{{ID: "host", Report: protocol.Report{Driver: "managed", State: "stopped"}}}}
	heartbeat := func() protocol.MachineDesired {
		t.Helper()
		data, _ := json.Marshal(report)
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("heartbeat %d %s", w.Code, w.Body.String())
		}
		var desired protocol.MachineDesired
		if err := json.Unmarshal(w.Body.Bytes(), &desired); err != nil {
			t.Fatal(err)
		}
		return desired
	}
	desired := heartbeat()
	if len(desired.Servers) != 1 || desired.Servers[0].GameUpdate != nil || desired.Servers[0].State != "stopped" {
		t.Fatalf("cancelled update sent %+v", desired)
	}
	state, err = a.workspace(ctx)
	if err != nil || !state.Hosts[0].Actions.UpdateGame {
		t.Fatalf("not retryable %+v %v", state.Hosts, err)
	}
	revision := state.Hosts[0].Revision
	if code := call("/api/hosts/host/game-update", a.AdminToken, fmt.Sprintf(`{"revision":%d}`, revision)); code != 202 {
		t.Fatalf("retry %d", code)
	}
	desired = heartbeat()
	if desired.Servers[0].GameUpdate == nil {
		t.Fatal("missing issued update")
	}
	state, err = a.workspace(ctx)
	if err != nil || state.Hosts[0].Actions.CancelGameUpdate {
		t.Fatalf("issued command cancellable %+v %v", state.Hosts, err)
	}
	if code := call("/api/hosts/host/game-update/cancel", a.AdminToken, fmt.Sprintf(`{"revision":%d}`, state.Hosts[0].Revision)); code != 409 {
		t.Fatalf("cancel after issuance %d", code)
	}
}

func TestUpdateDeliveryAndCancellationAreExclusive(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := "agent-secret"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,driver,supports_game_update) VALUES('machine','Machine','localhost',$1,now(),'managed',true)`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen) VALUES('host','Host','localhost',27015,'','machine','managed','stopped','stopped',now()); INSERT INTO game_updates(host_id,request_id,status) VALUES('host','ABCDEFGHIJKLMNOPQRSTUVWX23','pending')`); err != nil {
		t.Fatal(err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsGameUpdate: true, Servers: []protocol.ServerReport{{ID: "host", Report: protocol.Report{Driver: "managed", State: "stopped"}}}}
	data, _ := json.Marshal(report)
	start := make(chan struct{})
	codes := make(chan int, 1)
	deliveries := make(chan protocol.MachineDesired, 1)
	errs := make(chan int, 1)
	go func() {
		<-start
		req := httptest.NewRequest("POST", "/api/hosts/host/game-update/cancel", strings.NewReader(`{"revision":1}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		codes <- w.Code
	}()
	go func() {
		<-start
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			errs <- w.Code
			return
		}
		var desired protocol.MachineDesired
		_ = json.Unmarshal(w.Body.Bytes(), &desired)
		deliveries <- desired
	}()
	close(start)
	code := <-codes
	var desired protocol.MachineDesired
	select {
	case desired = <-deliveries:
	case failure := <-errs:
		t.Fatalf("heartbeat %d", failure)
	}
	if len(desired.Servers) != 1 {
		t.Fatalf("missing server %+v", desired)
	}
	switch code {
	case 204:
		if desired.Servers[0].GameUpdate != nil {
			t.Fatal("cancelled update was delivered")
		}
	case 409:
		if desired.Servers[0].GameUpdate == nil {
			t.Fatal("cancel rejected without issuance")
		}
	default:
		t.Fatalf("unexpected cancel status %d", code)
	}
}

func TestOldPendingUpdatesAreConservativelyMarkedIssued(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,''); ALTER TABLE game_updates DROP CONSTRAINT game_updates_status_check; ALTER TABLE game_updates ADD CONSTRAINT game_updates_status_check CHECK (status IN ('pending','complete','failed')); ALTER TABLE game_updates DROP COLUMN issued_at; INSERT INTO game_updates(host_id,request_id,status) VALUES('host','ABCDEFGHIJKLMNOPQRSTUVWX23','pending'); DELETE FROM schema_migrations WHERE version=49`); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var issued bool
	if err := a.DB.QueryRow(ctx, `SELECT issued_at IS NOT NULL FROM game_updates WHERE host_id='host'`).Scan(&issued); err != nil || !issued {
		t.Fatalf("legacy update not fenced: %v %v", issued, err)
	}
}
