package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"

	"clutchbase/internal/protocol"
	"testing"
)

func TestRestartStoragePersistsAndEnforcesStates(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash) VALUES('restart-host','Server','127.0.0.1',27015,'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES('restart-host','request','stopping')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE server_restarts SET phase='unverified' WHERE host_id='restart-host'`); err == nil {
		t.Fatal("accepted invalid phase")
	}
	var phase string
	if err := a.DB.QueryRow(ctx, `SELECT phase FROM server_restarts WHERE host_id='restart-host'`).Scan(&phase); err != nil || phase != "stopping" {
		t.Fatalf("state not preserved: %q %v", phase, err)
	}
}

func TestRestartAcknowledgementRequiresMatchingStoppedRequest(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','127.0.0.1','secret'); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,last_seen) VALUES('host','Host','127.0.0.1',27015,'x','machine','stopped','stopped',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES('host',$1,'stopping')`, id); err != nil {
		t.Fatal(err)
	}
	ack := func(capable bool, requestID, phase, state string) {
		t.Helper()
		tx, err := a.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = acknowledgeRestart(ctx, tx, "host", "machine", capable, protocol.Report{RestartID: requestID, RestartPhase: phase}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var got string
		if err = a.DB.QueryRow(ctx, `SELECT phase FROM server_restarts WHERE host_id='host'`).Scan(&got); err != nil || got != state {
			t.Fatalf("phase=%q want %q: %v", got, state, err)
		}
	}
	ack(false, id, "stopped", "stopping")
	ack(true, "BBBBBBBBBBBBBBBBBBBBBBBBBB", "stopped", "stopping")
	ack(true, id, "complete", "stopping")
	ack(true, id, "stopped", "starting")
	var desired string
	if err := a.DB.QueryRow(ctx, `SELECT desired_state FROM hosts WHERE id='host'`).Scan(&desired); err != nil || desired != "running" {
		t.Fatalf("desired=%q %v", desired, err)
	}
	ack(true, id, "stopped", "starting")
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET desired_state='stopped',observed_state='stopped' WHERE id='host'; UPDATE server_restarts SET phase='cancelling' WHERE host_id='host'`); err != nil {
		t.Fatal(err)
	}
	ack(true, id, "complete", "cancelling")
	ack(true, id, "cancelled", "cancelled")
	ack(true, id, "stopped", "cancelled")
	if err := a.DB.QueryRow(ctx, `SELECT desired_state FROM hosts WHERE id='host'`).Scan(&desired); err != nil || desired != "stopped" {
		t.Fatalf("cancelled restart changed desired state: %q %v", desired, err)
	}
}

func TestAdminStopCancelsPendingRestart(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','127.0.0.1','secret'); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen) VALUES('host','Host','127.0.0.1',27015,'x','machine','stopped','stopped','managed',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES('host',$1,'stopping')`, id); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := a.DB.QueryRow(ctx, `SELECT revision FROM hosts WHERE id='host'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	call := func(body string) int {
		req := httptest.NewRequest("PATCH", "/api/hosts/host", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if status := call(fmt.Sprintf(`{"revision":%d,"desiredState":"running"}`, revision)); status != 409 {
		t.Fatalf("conflicting start: %d", status)
	}
	if status := call(fmt.Sprintf(`{"revision":%d,"desiredState":"stopped"}`, revision)); status != 200 {
		t.Fatalf("stop: %d", status)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = acknowledgeRestart(ctx, tx, "host", "machine", true, protocol.Report{RestartID: id, RestartPhase: "stopped"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var phase, desired string
	if err = a.DB.QueryRow(ctx, `SELECT r.phase,h.desired_state FROM server_restarts r JOIN hosts h ON h.id=r.host_id WHERE h.id='host'`).Scan(&phase, &desired); err != nil || phase != "cancelling" || desired != "stopped" {
		t.Fatalf("stop lost to late ack: %s %s %v", phase, desired, err)
	}
}

func TestRestartHeartbeatFencesAcknowledgementsAndOldAgents(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','127.0.0.1',$1)`, digest("machine-secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen) VALUES('host','Host','127.0.0.1',27015,'x','machine','stopped','stopped','managed',now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES('host',$1,'stopping')`, id); err != nil {
		t.Fatal(err)
	}
	call := func(capable bool, report protocol.Report) protocol.MachineDesired {
		t.Helper()
		data, err := json.Marshal(protocol.MachineReport{Driver: "managed", SupportsRestart: capable, Servers: []protocol.ServerReport{{ID: "host", Report: report}}})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer machine-secret")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("heartbeat %d: %s", w.Code, w.Body.String())
		}
		var out protocol.MachineDesired
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Servers) != 1 {
			t.Fatalf("bad heartbeat: %+v %v", out, err)
		}
		return out
	}
	base := protocol.Report{Driver: "managed", State: "stopped"}
	old := call(false, base)
	if old.Servers[0].State != "stopped" || old.Servers[0].Restart != nil {
		t.Fatalf("old agent received restart: %+v", old)
	}
	out := call(true, base)
	if out.Servers[0].Restart == nil || out.Servers[0].Restart.Phase != "stop" {
		t.Fatalf("missing stop: %+v", out)
	}
	stale := base
	stale.RestartID = "BBBBBBBBBBBBBBBBBBBBBBBBBB"
	stale.RestartPhase = "stopped"
	out = call(true, stale)
	if out.Servers[0].State != "stopped" || out.Servers[0].Restart.Phase != "stop" {
		t.Fatalf("stale report advanced: %+v", out)
	}
	stopped := base
	stopped.RestartID = id
	stopped.RestartPhase = "stopped"
	out = call(true, stopped)
	if out.Servers[0].State != "running" || out.Servers[0].Restart == nil || out.Servers[0].Restart.Phase != "start" {
		t.Fatalf("matching acknowledgement not advanced: %+v", out)
	}
	old = call(false, base)
	if old.Servers[0].State != "stopped" || old.Servers[0].Restart != nil {
		t.Fatalf("downgrade bypassed fence: %+v", old)
	}
	running := protocol.Report{Driver: "managed", State: "running", RestartID: id, RestartPhase: "complete"}
	out = call(true, running)
	if out.Servers[0].State != "running" || out.Servers[0].Restart != nil {
		t.Fatalf("completed restart retained command: %+v", out)
	}
	var phase string
	if err := a.DB.QueryRow(ctx, `SELECT phase FROM server_restarts WHERE host_id='host'`).Scan(&phase); err != nil || phase != "complete" {
		t.Fatalf("restart completion: %s %v", phase, err)
	}
}

func TestRestartRequestRequiresFreshCapabilityAndNoActiveRoom(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_restart) VALUES('machine','Machine','127.0.0.1','secret','managed',now(),false); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen) VALUES('host','Host','127.0.0.1',27015,'x','machine','running','running','managed',now())`); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := a.DB.QueryRow(ctx, `SELECT revision FROM hosts WHERE id='host'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	call := func(token string, rev int64) int {
		req := httptest.NewRequest("POST", "/api/hosts/host/restart", strings.NewReader(fmt.Sprintf(`{"revision":%d}`, rev)))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := call("", revision); code != 401 {
		t.Fatalf("unauthorized: %d", code)
	}
	if code := call(a.AdminToken, revision); code != 409 {
		t.Fatalf("old agent accepted: %d", code)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE machines SET supports_restart=true WHERE id='machine'`); err != nil {
		t.Fatal(err)
	}
	if code := call(a.AdminToken, revision+1); code != 409 {
		t.Fatalf("stale revision accepted: %d", code)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Match','de_mirage','open')`); err != nil {
		t.Fatal(err)
	}
	if code := call(a.AdminToken, revision); code != 409 {
		t.Fatalf("open room accepted: %d", code)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'`); err != nil {
		t.Fatal(err)
	}
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- call(a.AdminToken, revision) }()
	}
	first, second := <-results, <-results
	if !((first == 202 && second == 409) || (first == 409 && second == 202)) {
		t.Fatalf("concurrent restart requests: %d, %d", first, second)
	}
	var phase, desired, id string
	if err := a.DB.QueryRow(ctx, `SELECT r.phase,h.desired_state,r.request_id FROM server_restarts r JOIN hosts h ON h.id=r.host_id WHERE h.id='host'`).Scan(&phase, &desired, &id); err != nil || phase != "stopping" || desired != "stopped" || !protocol.ValidCommandID(id) {
		t.Fatalf("bad request: %s %s %s %v", phase, desired, id, err)
	}
	workspace, err := a.workspace(ctx)
	if err != nil || len(workspace.Hosts) != 1 || workspace.Hosts[0].RestartPhase != "stopping" || !workspace.Hosts[0].Actions.CancelRestart || workspace.Hosts[0].Actions.Start {
		t.Fatalf("incorrect restart projection: %+v %v", workspace.Hosts, err)
	}
	if code := call(a.AdminToken, revision); code != 409 {
		t.Fatalf("duplicate revision accepted: %d", code)
	}
}

func TestRestartUnsafeStartCancelsAgentFence(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','127.0.0.1','secret'); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen) VALUES('host','Host','127.0.0.1',27015,'x','machine','stopped','stopped','managed',now()); INSERT INTO match_rooms(id,host_id,title,map,status) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Match','de_mirage','open')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO server_restarts(host_id,request_id,phase) VALUES('host',$1,'stopping')`, id); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = acknowledgeRestart(ctx, tx, "host", "machine", true, protocol.Report{RestartID: id, RestartPhase: "stopped"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var phase, desired string
	if err = a.DB.QueryRow(ctx, `SELECT r.phase,h.desired_state FROM server_restarts r JOIN hosts h ON h.id=r.host_id WHERE h.id='host'`).Scan(&phase, &desired); err != nil || phase != "cancelling" || desired != "stopped" {
		t.Fatalf("unsafe start not cancelled: %s %s %v", phase, desired, err)
	}
}
