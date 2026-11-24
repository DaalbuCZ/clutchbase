package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestMatchPauseCommandRequiresLiveEvidenceAndAcknowledgement(t *testing.T) {
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	a := integrationAPI(t)
	ctx := context.Background()
	admin := strings.Repeat("a", 32)
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_match_control,supports_presets,supports_whitelist)
		VALUES ('machine','Machine','localhost',$1,now(),true,true,true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,access_status,active_preset,match_phase,can_pause_match,active_match_setup_id,last_seen,revision)
		VALUES ('server','Server','localhost',27015,'','machine','running','running','managed','verified','match','live',true,$1,now(),1)`, setup); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, token string, body any, want int, out any) {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if out != nil && json.Unmarshal(res.Body.Bytes(), out) != nil {
			t.Fatalf("invalid JSON: %s", res.Body.String())
		}
	}
	call(http.MethodPost, "/api/hosts/server/match-control", admin, map[string]any{"action": "resume", "revision": 1}, http.StatusConflict, nil)
	call(http.MethodPost, "/api/hosts/server/match-control", admin, map[string]any{"action": "pause", "revision": 0}, http.StatusConflict, nil)
	call(http.MethodPost, "/api/hosts/server/match-control", "invalid", map[string]any{"action": "pause", "revision": 1}, http.StatusUnauthorized, nil)
	var pending protocol.Host
	call(http.MethodPost, "/api/hosts/server/match-control", admin, map[string]any{"action": "pause", "revision": 1}, http.StatusAccepted, &pending)
	if pending.MatchControlStatus != "pending" || pending.MatchPhase != "live" || pending.Actions.PauseMatch || pending.Actions.ResumeMatch {
		t.Fatalf("unexpected pending projection: %+v", pending)
	}
	var desired protocol.MachineDesired
	report := protocol.MachineReport{Driver: "managed", SupportsMatchControl: true, SupportsPresets: true, SupportsWhitelist: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", MatchPhase: "live", CanPauseMatch: true, SupportsFencedMatchControl: true, ActiveMatchSetupID: setup}}}}
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	if len(desired.Servers) != 1 || desired.Servers[0].MatchControl == nil || desired.Servers[0].MatchControl.Action != "pause" || desired.Servers[0].MatchControl.SetupID != setup {
		t.Fatalf("missing pause request: %+v", desired)
	}
	command := desired.Servers[0].MatchControl
	report.Servers[0].Report.MatchControlID = command.ID
	report.Servers[0].Report.MatchControlAction = "resume"
	report.Servers[0].Report.MatchControlStatus = "complete"
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	var status string
	if err := a.DB.QueryRow(ctx, "SELECT match_control_status FROM hosts WHERE id='server'").Scan(&status); err != nil || status != "pending" {
		t.Fatalf("mismatched acknowledgement accepted: %s %v", status, err)
	}
	report.Servers[0].Report.MatchControlAction = "pause"
	report.Servers[0].Report.MatchPhase = "paused"
	report.Servers[0].Report.CanPauseMatch = false
	report.Servers[0].Report.CanResumeMatch = true
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	var workspace protocol.Workspace
	call(http.MethodGet, "/api/state", admin, nil, http.StatusOK, &workspace)
	if len(workspace.Hosts) != 1 || workspace.Hosts[0].MatchControlStatus != "complete" || workspace.Hosts[0].MatchPhase != "paused" || !workspace.Hosts[0].Actions.ResumeMatch || workspace.Hosts[0].Actions.PauseMatch {
		t.Fatalf("wrong acknowledged pause state: %+v", workspace.Hosts)
	}
	call(http.MethodPost, "/api/hosts/server/match-control", admin, map[string]any{"action": "resume", "revision": workspace.Hosts[0].Revision}, http.StatusAccepted, &pending)
	if pending.MatchControlStatus != "pending" || pending.MatchPhase != "paused" {
		t.Fatalf("resume changed state optimistically: %+v", pending)
	}
	var stopped protocol.Host
	call(http.MethodPatch, "/api/hosts/server", admin, map[string]any{"desiredState": "stopped", "revision": pending.Revision}, http.StatusOK, &stopped)
	if stopped.MatchControlStatus != "failed" || stopped.Actions.PauseMatch || stopped.Actions.ResumeMatch {
		t.Fatalf("stopping did not retire the pending control: %+v", stopped)
	}
	if err := a.DB.QueryRow(ctx, "SELECT match_control_status FROM hosts WHERE id='server'").Scan(&status); err != nil || status != "failed" {
		t.Fatalf("stop did not persist the failed control: %s %v", status, err)
	}
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET desired_state='running', match_control_status='' WHERE id='server'"); err != nil {
		t.Fatal(err)
	}
	report.Servers[0].Report.MatchControlID = ""
	report.Servers[0].Report.MatchControlAction = ""
	report.Servers[0].Report.MatchControlStatus = ""
	report.Servers[0].Report.SupportsFencedMatchControl = false
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	call(http.MethodGet, "/api/state", admin, nil, http.StatusOK, &workspace)
	if workspace.Hosts[0].Actions.ResumeMatch {
		t.Fatal("old reporter advertised resume")
	}
	report.Servers[0].Report.SupportsFencedMatchControl = true
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	call(http.MethodGet, "/api/state", admin, nil, http.StatusOK, &workspace)
	call(http.MethodPost, "/api/hosts/server/match-control", admin, map[string]any{"action": "resume", "revision": workspace.Hosts[0].Revision}, http.StatusAccepted, &pending)
	report.Servers[0].Report.ActiveMatchSetupID = "ABCDEFGHIJKLMNOPQRSTUVWX24"
	desired = protocol.MachineDesired{}
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	if desired.Servers[0].MatchControl != nil {
		t.Fatal("delayed command delivered to another setup")
	}
	if err := a.DB.QueryRow(ctx, "SELECT match_control_status FROM hosts WHERE id='server'").Scan(&status); err != nil || status != "failed" {
		t.Fatalf("changed setup did not fail the request: %s %v", status, err)
	}
}
