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

func TestLiveModeCommandRequiresRuntimeAndAcknowledges(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := strings.Repeat("a", 32)
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_live_mode,supports_presets,supports_whitelist)
		VALUES ('machine','Machine','localhost',$1,now(),true,true,true)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,access_status,active_preset,can_live_mode,last_seen,revision)
		VALUES ('server','Server','localhost',27015,'','machine','running','running','managed','verified','match',true,now(),1)`); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, auth string, body any, want int, out any) {
		t.Helper()
		data, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer "+auth)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if out != nil && json.Unmarshal(res.Body.Bytes(), out) != nil {
			t.Fatal("invalid JSON response", res.Body.String())
		}
	}
	var pending protocol.Host
	call(http.MethodPost, "/api/hosts/server/mode", token, map[string]any{"mode": "practice", "revision": 1}, http.StatusAccepted, &pending)
	if pending.ModeCommandStatus != "pending" || pending.ModeCommandMode != "practice" || pending.Actions.SwitchMode {
		t.Fatalf("mode command was not projected as pending: %+v", pending)
	}
	var desired protocol.MachineDesired
	report := protocol.MachineReport{Driver: "managed", SupportsLiveMode: true, SupportsPresets: true, SupportsWhitelist: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", CanLiveMode: true}}}}
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	if len(desired.Servers) != 1 || desired.Servers[0].LiveMode == nil || desired.Servers[0].LiveMode.Mode != "practice" || len(desired.Servers[0].LiveMode.ID) != 26 {
		t.Fatalf("pending live mode was not delivered: %+v", desired)
	}
	command := desired.Servers[0].LiveMode
	report.Servers[0].Report.ActivePreset = "practice"
	report.Servers[0].Report.ModeCommandID = command.ID
	report.Servers[0].Report.ModeCommandStatus = "complete"
	report.Servers[0].Report.CanLiveMode = false // Actual practice mode has no active match phase.
	call(http.MethodPost, "/api/machine-agent/machine/heartbeat", "agent-token", report, http.StatusOK, &desired)
	var host protocol.Host
	call(http.MethodGet, "/api/state", token, nil, http.StatusOK, &protocol.Workspace{})
	if err := a.DB.QueryRow(ctx, `SELECT mode_command_status,active_preset,can_live_mode FROM hosts WHERE id='server'`).Scan(&host.ModeCommandStatus, &host.ActivePreset, &host.CanLiveMode); err != nil {
		t.Fatal(err)
	}
	if host.ModeCommandStatus != "complete" || host.ActivePreset != "practice" || host.CanLiveMode {
		t.Fatalf("runtime acknowledgement was not saved: %+v", host)
	}
}
