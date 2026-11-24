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

func TestLateMatchLoadNeedsCurrentMatchingRuntimeEvidence(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const other = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state) VALUES('server','Server','localhost',27015,'','machine','running');
		INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_error,load_command_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Original','de_mirage','closed','failed','Timed out','ABCDEFGHIJKLMNOPQRSTUVWX23');
		INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','server','New','de_mirage','open','','')`); err != nil {
		t.Fatal(err)
	}
	report := protocol.MachineReport{Driver: "managed", SupportsMatchSetup: true, SupportsMatchControl: true, SupportsMapObservation: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", ActiveMap: "de_mirage", MatchPhase: "setup", MatchSetupID: setup, MatchSetupStatus: "complete"}}}}
	heartbeat := func() {
		t.Helper()
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
	}
	check := func(status string, blocked bool) {
		t.Helper()
		var gotStatus, gotError string
		if err := a.DB.QueryRow(ctx, "SELECT load_status,load_error FROM match_rooms WHERE id=$1", setup).Scan(&gotStatus, &gotError); err != nil {
			t.Fatal(err)
		}
		if gotStatus != status || (status == "complete" && gotError != "") {
			t.Fatalf("load state %q %q, want %q", gotStatus, gotError, status)
		}
		rooms, err := a.matches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, room := range rooms {
			if room.ID == other {
				found = true
				if room.LoadBlockedByFailure != blocked {
					t.Fatalf("incorrect server block: %+v", room)
				}
			}
		}
		if !found {
			t.Fatal("new room missing")
		}
	}
	heartbeat()
	check("failed", true)
	report.Servers[0].ActiveMatchSetupID = other
	heartbeat()
	check("failed", true)
	report.Servers[0].ActiveMatchSetupID = setup
	report.SupportsMatchControl = false
	heartbeat()
	check("failed", true)
	report.SupportsMatchControl = true
	report.Servers[0].CanLoadMatch = true
	heartbeat()
	check("failed", true)
	report.Servers[0].CanLoadMatch = false
	// The plugin may have timed out before MatchZy loaded the file.
	report.Servers[0].MatchSetupStatus = "failed"
	report.Servers[0].MatchSetupError = "Load timed out"
	report.Servers[0].MatchSetupID = other
	heartbeat()
	check("complete", false)
	var status string
	if err := a.DB.QueryRow(ctx, "SELECT status FROM match_rooms WHERE id=$1", setup).Scan(&status); err != nil || status != "closed" {
		t.Fatalf("closed room reopened: %q %v", status, err)
	}
}
