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

func TestReporterUpdateWaitsForMatchingStoppedAgentEvidence(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_reporter_update) VALUES('machine','Machine','localhost',$1,now(),true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen,revision,reporter_source_hash,reporter_installed_hash) VALUES('server','Server','localhost',27015,'','machine','stopped','stopped','managed',now(),1,$1,$2)`, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
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
	path := "/api/hosts/server/reporter-update"
	call("POST", path, "bad", map[string]any{"revision": 1}, http.StatusUnauthorized, nil)
	call("POST", path, a.AdminToken, map[string]any{"revision": 0}, http.StatusConflict, nil)
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET desired_state='running',observed_state='running' WHERE id='server'"); err != nil {
		t.Fatal(err)
	}
	call("POST", path, a.AdminToken, map[string]any{"revision": 1}, http.StatusConflict, nil)
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET desired_state='stopped',observed_state='stopped' WHERE id='server'; UPDATE machines SET supports_reporter_update=false WHERE id='machine'"); err != nil {
		t.Fatal(err)
	}
	call("POST", path, a.AdminToken, map[string]any{"revision": 1}, http.StatusConflict, nil)
	if _, err := a.DB.Exec(ctx, "UPDATE machines SET supports_reporter_update=true WHERE id='machine'; UPDATE hosts SET reporter_installed_hash='' WHERE id='server'"); err != nil {
		t.Fatal(err)
	}
	var pending protocol.Host
	call("POST", path, a.AdminToken, map[string]any{"revision": 1}, http.StatusAccepted, &pending)
	if pending.ReporterUpdateStatus != "pending" || pending.Actions.Start || pending.Actions.UpdateReporter {
		t.Fatalf("unsafe pending state: %+v", pending)
	}
	call("PATCH", "/api/hosts/server", a.AdminToken, map[string]any{"revision": pending.Revision, "desiredState": "running"}, http.StatusConflict, nil)
	report := protocol.MachineReport{Driver: "managed", SupportsReporterUpdate: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "stopped", ReporterSourceHash: strings.Repeat("a", 64)}}}}
	var desired protocol.MachineDesired
	heartbeat := func() {
		t.Helper()
		call("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	}
	heartbeat()
	if len(desired.Servers) != 1 || desired.Servers[0].ReporterUpdate == nil || desired.Servers[0].ReporterUpdate.Hash != strings.Repeat("a", 64) {
		t.Fatalf("missing fixed reporter request: %+v", desired)
	}
	command := desired.Servers[0].ReporterUpdate
	report.Servers[0].ReporterUpdateID = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	report.Servers[0].ReporterUpdateStatus = "complete"
	report.Servers[0].ReporterInstalledHash = command.Hash
	heartbeat()
	var status string
	if err := a.DB.QueryRow(ctx, "SELECT reporter_command_status FROM hosts WHERE id='server'").Scan(&status); err != nil || status != "pending" {
		t.Fatalf("unrelated acknowledgement accepted: %s %v", status, err)
	}
	report.Servers[0].ReporterUpdateID = command.ID
	report.Servers[0].ReporterInstalledHash = strings.Repeat("b", 64)
	heartbeat()
	if err := a.DB.QueryRow(ctx, "SELECT reporter_command_status FROM hosts WHERE id='server'").Scan(&status); err != nil || status != "pending" {
		t.Fatalf("wrong installed fingerprint accepted: %s %v", status, err)
	}
	report.Servers[0].ReporterInstalledHash = command.Hash
	heartbeat()
	var workspace protocol.Workspace
	call("GET", "/api/state", a.AdminToken, nil, http.StatusOK, &workspace)
	if len(workspace.Hosts) != 1 || workspace.Hosts[0].ReporterUpdateStatus != "complete" || !workspace.Hosts[0].Actions.Start || workspace.Hosts[0].ReporterUpdateAvailable {
		t.Fatalf("incorrect completed state: %+v", workspace.Hosts)
	}
}
