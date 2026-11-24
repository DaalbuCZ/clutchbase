package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestEndMatchTargetsAcknowledgedSetup(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	_, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_match_control) VALUES('machine','Machine','localhost',$1,now(),true)`, digest("agent-token"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,access_status,active_preset,match_phase,active_match_setup_id,last_seen,revision) VALUES('server','Server','localhost',27015,'','machine','running','running','managed','verified','match','live',$1,now(),1)`, setup)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,load_status,load_command_id) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','server','Match','de_mirage','complete',$1)`, setup)
	if err != nil {
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
			t.Fatalf("%s %s: got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		if out != nil {
			if err := json.Unmarshal(res.Body.Bytes(), out); err != nil {
				t.Fatal(err)
			}
		}
	}
	admin := strings.Repeat("a", 32)
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || !rooms[0].CanEnd {
		t.Fatalf("not endable: %+v %v", rooms, err)
	}
	if _, err = a.DB.Exec(ctx, "UPDATE machines SET supports_match_control=false WHERE id='machine'"); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/end", admin, map[string]any{"revision": 1}, 409, nil)
	if _, err = a.DB.Exec(ctx, "UPDATE machines SET supports_match_control=true WHERE id='machine'; UPDATE hosts SET active_match_setup_id='' WHERE id='server'"); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/end", admin, map[string]any{"revision": 1}, 409, nil)
	if _, err = a.DB.Exec(ctx, "UPDATE hosts SET active_match_setup_id=$1 WHERE id='server'", setup); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/end", "bad", map[string]any{"revision": 1}, 401, nil)
	call("POST", "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/end", admin, map[string]any{"revision": 0}, 409, nil)
	call("POST", "/api/matches/BCDEFGHIJKLMNOPQRSTUVWXY23/end", admin, map[string]any{"revision": 1}, 202, nil)
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].CanEnd || rooms[0].Status != "open" || rooms[0].EndedAt != nil {
		t.Fatalf("closed optimistically: %+v %v", rooms, err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchControl: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", MatchPhase: "live", ActiveMatchSetupID: setup}}}}
	var desired protocol.MachineDesired
	call("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, 200, &desired)
	if len(desired.Servers) != 1 || desired.Servers[0].MatchControl == nil || desired.Servers[0].MatchControl.Action != "end" || desired.Servers[0].MatchControl.SetupID != setup {
		t.Fatalf("wrong command: %+v", desired)
	}
	cmd := desired.Servers[0].MatchControl
	report.Servers[0].MatchControlID = "ZYXWVUTSRQPONMLKJIHGFEDC23"
	report.Servers[0].MatchControlAction = "end"
	report.Servers[0].MatchControlStatus = "complete"
	call("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, 200, &desired)
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].EndedAt != nil {
		t.Fatalf("unrelated acknowledgement closed room: %+v %v", rooms, err)
	}
	report.Servers[0].MatchControlID = cmd.ID
	report.Servers[0].ActiveMatchSetupID = ""
	report.Servers[0].MatchPhase = "idle"
	call("POST", "/api/machine-agent/machine/heartbeat", "agent-token", report, 200, &desired)
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].EndedAt == nil || rooms[0].Status != "closed" {
		t.Fatalf("acknowledgement did not close room: %+v %v", rooms, err)
	}
	const player = "76561198154367261"
	if _, err = a.DB.Exec(ctx, "UPDATE hosts SET whitelist=ARRAY[$1]::text[] WHERE id='server'", player); err != nil {
		t.Fatal(err)
	}
	playerRooms, err := a.playerMatches(ctx, player)
	if err != nil || len(playerRooms) != 1 || playerRooms[0].EndedAt == nil || !playerRooms[0].EndedAt.Equal(*rooms[0].EndedAt) || playerRooms[0].Status != "closed" {
		t.Fatalf("player did not see the confirmed match end: %+v %v", playerRooms, err)
	}
}
