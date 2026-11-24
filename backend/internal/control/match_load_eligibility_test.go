package control

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"clutchbase/internal/protocol"
)

func TestMatchLoadBlockersMatchLockedEligibility(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	_, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation,supports_match_setup)
		VALUES('machine','Machine','localhost','agent','managed',now(),true,true)`)
	if err == nil {
		_, err = a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,desired_state,observed_state,last_seen,access_status,active_preset,active_map,can_load_match,match_phase)
			VALUES('host','Host','localhost',27015,'','machine','managed','running','running',now(),'verified','match','de_mirage',true,'idle')`)
	}
	if err == nil {
		_, err = a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,players_per_side) VALUES($1,'host','Friends','de_mirage',1)`, room)
	}
	if err == nil {
		_, err = a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262')`)
	}
	if err == nil {
		_, err = a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES($1,'76561198154367261','ct',true),($1,'76561198154367262','t',true)`, room)
	}
	if err != nil {
		t.Fatal(err)
	}
	check := func(name, update, blocker string) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, update); err != nil {
			t.Fatalf("%s setup: %v", name, err)
		}
		matches, err := a.matches(ctx)
		if err != nil || len(matches) == 0 {
			t.Fatalf("%s snapshot: %+v, %v", name, matches, err)
		}
		var m protocol.Match
		found := false
		for _, candidate := range matches {
			if candidate.ID == room {
				m, found = candidate, true
				break
			}
		}
		if !found {
			t.Fatalf("%s room missing from snapshot: %+v", name, matches)
		}
		if m.CanLoad || !slices.Contains(m.LoadBlockers, blocker) {
			t.Fatalf("%s snapshot disagrees: canLoad=%v blockers=%v", name, m.CanLoad, m.LoadBlockers)
		}
		req := httptest.NewRequest("POST", "/api/matches/"+room+"/load", nil)
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != 409 {
			t.Fatalf("%s locked load status %d: %s", name, res.Code, res.Body.String())
		}
	}
	checks := []struct{ name, update, blocker string }{
		{"null heartbeat", `UPDATE hosts SET last_seen=NULL WHERE id='host'`, "server_offline"},
		{"reporter", `UPDATE hosts SET last_seen=now(),can_load_match=false WHERE id='host'`, "reporter_unsupported"},
		{"manual ready", `UPDATE hosts SET can_load_match=true,supports_manual_ready=false WHERE id='host'; UPDATE match_rooms SET manual_ready=true WHERE id='` + room + `'`, "reporter_unsupported"},
		{"phase", `UPDATE match_rooms SET manual_ready=false WHERE id='` + room + `'; UPDATE hosts SET supports_manual_ready=true,match_phase='setup' WHERE id='host'`, "server_not_ready"},
		{"server error", `UPDATE hosts SET match_phase='idle',error='query failed' WHERE id='host'`, "server_not_ready"},
		{"previous failure", `UPDATE hosts SET error=''; INSERT INTO match_rooms(id,host_id,title,map,status,load_status,players_per_side) VALUES('ZYXWVUTSRQPONMLKJIHGFEDC32','host','Previous','de_nuke','closed','failed',1)`, "server_not_ready"},
		{"Workshop", `DELETE FROM match_rooms WHERE id='ZYXWVUTSRQPONMLKJIHGFEDC32'; UPDATE match_rooms SET series_maps=ARRAY['de_mirage','workshop/123/de_custom'],series_sides=ARRAY['team1_ct','team1_t'] WHERE id='` + room + `'`, "workshop_unsupported"},
		{"roster readiness", `UPDATE match_rooms SET series_maps=ARRAY[]::text[],series_sides=ARRAY[]::text[] WHERE id='` + room + `'; UPDATE match_room_players SET ready=false WHERE match_id='` + room + `' AND side='t'`, "players_not_ready"},
	}
	for _, c := range checks {
		check(c.name, c.update, c.blocker)
		// Restore a single fully eligible setup before applying the next independent gate.
		if _, err := a.DB.Exec(ctx, `DELETE FROM match_rooms WHERE id<>'`+room+`'; UPDATE hosts SET last_seen=now(),desired_state='running',observed_state='running',driver='managed',access_status='verified',active_preset='match',active_map='de_mirage',can_load_match=true,match_phase='idle',error='',remove_requested=false,active_match_setup_id='',supports_manual_ready=true,match_control_status='',mode_command_status=''; UPDATE match_rooms SET manual_ready=false,series_maps=ARRAY[]::text[],series_sides=ARRAY[]::text[] WHERE id='`+room+`'; UPDATE match_room_players SET ready=true WHERE match_id='`+room+`'`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET error='query failed' WHERE id='host'; INSERT INTO match_rooms(id,host_id,title,map,status,load_status,players_per_side) VALUES('ZYXWVUTSRQPONMLKJIHGFEDC32','host','Previous','de_nuke','closed','failed',1)`); err != nil {
		t.Fatal(err)
	}
	combined, err := a.matches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range combined {
		if match.ID == room {
			count := 0
			for _, blocker := range match.LoadBlockers {
				if blocker == "server_not_ready" {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("expected one server blocker, got %v", match.LoadBlockers)
			}
		}
	}
	if _, err := a.DB.Exec(ctx, `DELETE FROM match_rooms WHERE id='ZYXWVUTSRQPONMLKJIHGFEDC32'; UPDATE hosts SET error='' WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	// Capacity is independently enforced by the endpoint and must also be visible in snapshots.
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367263')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES($1,'76561198154367263','ct',true)`, room); err != nil {
		t.Fatal(err)
	}
	matches, err := a.matches(ctx)
	if err != nil || len(matches) != 1 || matches[0].CanLoad || !slices.Contains(matches[0].LoadBlockers, "players_not_ready") {
		t.Fatalf("over-capacity roster snapshot: %+v, %v", matches, err)
	}
	req := httptest.NewRequest("POST", "/api/matches/"+room+"/load", nil)
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, req)
	if res.Code != 409 {
		t.Fatalf("over-capacity locked load status %d", res.Code)
	}
}
