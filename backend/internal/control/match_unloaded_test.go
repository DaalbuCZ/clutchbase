package control

import (
	"context"
	"net/http/httptest"
	"testing"

	"clutchbase/internal/protocol"
)

func TestUnloadedSetupRequiresFreshVerifiedIdleEvidence(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,whitelist,desired_state,observed_state,driver,access_status,active_preset,match_phase,can_load_match,last_seen) VALUES('server','Server','localhost',27015,'',ARRAY[$1]::text[],'running','running','managed','verified','match','idle',true,now())`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','server','Room','de_mirage','open','complete','ABCDEFGHIJKLMNOPQRSTUVWX23')`); err != nil {
		t.Fatal(err)
	}
	assert := func(want bool) {
		t.Helper()
		rooms, err := a.matches(ctx)
		if err != nil || len(rooms) != 1 || rooms[0].SetupUnloaded != want || rooms[0].Status != "open" || rooms[0].EndedAt != nil {
			t.Fatalf("manager: %+v %v", rooms, err)
		}
		players, err := a.playerMatches(ctx, steam)
		if err != nil || len(players) != 1 || players[0].SetupUnloaded != want {
			t.Fatalf("player: %+v %v", players, err)
		}
	}
	assert(true)
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	socket := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var state protocol.Workspace
	readLive(t, socket, &state)
	if !state.Matches[0].SetupUnloaded {
		t.Fatal("initial socket state missing")
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET match_phase='live',can_load_match=false,active_match_setup_id='ABCDEFGHIJKLMNOPQRSTUVWX23' WHERE id='server'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, socket, &state)
	if state.Matches[0].SetupUnloaded {
		t.Fatal("active match shown unloaded")
	}
	assert(false)
	mutations := []string{
		`UPDATE hosts SET match_phase='idle',can_load_match=true,active_match_setup_id='',last_seen=now()-interval '30 seconds' WHERE id='server'`,
		`UPDATE hosts SET last_seen=now(),access_status='blocked' WHERE id='server'`,
		`UPDATE hosts SET access_status='verified',error='agent error' WHERE id='server'`,
		`UPDATE hosts SET error='',match_control_status='pending' WHERE id='server'`,
		`UPDATE hosts SET match_control_status='',mode_command_status='pending' WHERE id='server'`,
		`UPDATE hosts SET mode_command_status='',can_load_match=false WHERE id='server'`,
		`UPDATE hosts SET can_load_match=true,active_match_setup_id='ABCDEFGHIJKLMNOPQRSTUVWX23' WHERE id='server'`,
		`UPDATE hosts SET active_match_setup_id='',desired_state='stopped' WHERE id='server'`,
	}
	for _, sql := range mutations {
		if _, err := a.DB.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
		assert(false)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET desired_state='running' WHERE id='server'`); err != nil {
		t.Fatal(err)
	}
	assert(true)
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed' WHERE id='ABCDEFGHIJKLMNOPQRSTUVWX23'`); err != nil {
		t.Fatal(err)
	}
	rooms, err := a.matches(ctx)
	if err != nil || rooms[0].SetupUnloaded {
		t.Fatalf("closed room: %+v %v", rooms, err)
	}
}
