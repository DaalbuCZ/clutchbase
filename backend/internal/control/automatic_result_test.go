package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestAutomaticResultIsScopedDurableAndIdempotent(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const token = "private-agent-token"
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const steam = "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,whitelist) VALUES('server','Server','localhost',27015,'','machine',ARRAY[$1]::text[])`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id,load_fence_required,result_ct,result_t) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','server','Room','de_mirage','open','complete',$1,true,15,13)`, setup); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, steam); err != nil {
		t.Fatal(err)
	}
	const otherSteam = "76561198154367262"
	const thirdSteam = "76561198154367263"
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, otherSteam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, thirdSteam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES('team-a','Alpha',$1),('team-b','Bravo',$2),('team-c','Charlie',$3)`, steam, otherSteam, thirdSteam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status,map_pool) VALUES('cup','Cup',1,'active',ARRAY['de_mirage','de_inferno','de_nuke','de_ancient','de_anubis','de_overpass','de_dust2'])`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','team-a','checked_in'),('cup','team-b','checked_in'),('cup','team-c','checked_in')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,room_id,team_b_side) VALUES('fixture','cup',1,1,'team-a','team-b','pending','BCDEFGHIJKLMNOPQRSTUVWXY23','ct')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_b,status) VALUES('final','cup',2,1,'team-c','pending')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side,kills,deaths,assists) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23',$1,'ct',20,10,5)`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed',reported_ct=13,reported_t=9,reported_at=now() WHERE id='BCDEFGHIJKLMNOPQRSTUVWXY23'`); err != nil {
		t.Fatal(err)
	}
	projected, err := a.tournaments(ctx)
	if err != nil || projected[0].Matches[0].CanReportWinner {
		t.Fatalf("a non-tied reported score exposed manual winner selection: %+v %v", projected, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET reported_ct=13,reported_t=13 WHERE id='BCDEFGHIJKLMNOPQRSTUVWXY23'`); err != nil {
		t.Fatal(err)
	}
	projected, err = a.tournaments(ctx)
	if err != nil || !projected[0].Matches[0].CanReportWinner {
		t.Fatalf("a tied reported score did not allow manual resolution: %+v %v", projected, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='open',reported_ct=NULL,reported_t=NULL,reported_at=NULL WHERE id='BCDEFGHIJKLMNOPQRSTUVWXY23'`); err != nil {
		t.Fatal(err)
	}
	result := &protocol.MatchResult{SetupID: setup, CT: 13, T: 9, Players: []protocol.ReportedPlayer{{SteamID: steam, Kills: 12, Deaths: 7, Assists: 3}}}
	report := protocol.MachineReport{Driver: "managed", Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "stopped", MatchResult: result}}}}
	call := func(r protocol.MachineReport, secret string, want int) protocol.MachineDesired {
		t.Helper()
		body, _ := json.Marshal(r)
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+secret)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("got %d want %d: %s", res.Code, want, res.Body.String())
		}
		var out protocol.MachineDesired
		if want == 200 {
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	call(report, "wrong", 401)
	invalid := *result
	invalid.CT = 100
	bad := report
	bad.Servers = append([]protocol.ServerReport(nil), report.Servers...)
	bad.Servers[0].MatchResult = &invalid
	call(bad, token, 400)
	invalidPlayer := *result
	invalidPlayer.Players = []protocol.ReportedPlayer{{SteamID: "76561198154367262", Kills: 2, Deaths: 1, Assists: 0}}
	bad.Servers[0].MatchResult = &invalidPlayer
	if reply := call(bad, token, 200); reply.Servers[0].ResultAck != nil {
		t.Fatal("unrostered player was acknowledged")
	}
	duplicate := *result
	duplicate.Players = []protocol.ReportedPlayer{result.Players[0], result.Players[0]}
	bad.Servers[0].MatchResult = &duplicate
	call(bad, token, 400)
	foreign := report
	foreign.Servers = append(append([]protocol.ServerReport(nil), report.Servers...), protocol.ServerReport{ID: "foreign", Report: protocol.Report{Driver: "managed", State: "stopped"}})
	call(foreign, token, 400)
	before, err := a.matches(ctx)
	if err != nil || before[0].ReportedCT != nil || before[0].Status != "open" {
		t.Fatalf("rejected report changed room: %+v %v", before, err)
	}
	reply := call(report, token, 200)
	if len(reply.Servers) != 1 || reply.Servers[0].ResultAck == nil || !reply.Servers[0].ResultAck.Equal(*result) {
		t.Fatalf("ack: %+v", reply)
	}
	rooms, err := a.matches(ctx)
	if err != nil || len(rooms) != 1 || rooms[0].Status != "closed" || rooms[0].ReportedCT == nil || *rooms[0].ReportedCT != 13 || *rooms[0].ReportedT != 9 || *rooms[0].ResultCT != 15 || rooms[0].ResultRevision != 1 {
		t.Fatalf("stored: %+v %v", rooms, err)
	}
	if rooms[0].Participants[0].ReportedStats == nil || rooms[0].Participants[0].ReportedStats.Kills != 12 || rooms[0].Participants[0].Stats.Kills != 20 {
		t.Fatalf("wrong player stats: %+v", rooms[0].Participants[0])
	}
	tournaments, err := a.tournaments(ctx)
	if err != nil || len(tournaments) != 1 || tournaments[0].Status != "active" || tournaments[0].WinnerTeamID != "" || tournaments[0].Matches[0].WinnerTeamID != "team-b" || tournaments[0].Matches[0].ReportedCT == nil || *tournaments[0].Matches[0].ReportedCT != 13 || tournaments[0].Matches[1].TeamA == nil || tournaments[0].Matches[1].TeamA.TeamID != "team-b" || tournaments[0].Matches[1].TeamB == nil || tournaments[0].Matches[1].TeamB.TeamID != "team-c" || tournaments[0].Matches[1].VetoTurnTeamID != "team-b" {
		t.Fatalf("reported result did not advance and project the bracket: %+v %v", tournaments, err)
	}
	player, err := a.playerMatches(ctx, steam)
	if err != nil || len(player) != 1 || player[0].ReportedCT == nil || *player[0].ReportedCT != 13 {
		t.Fatalf("player: %+v %v", player, err)
	}
	if player[0].Participants[0].ReportedStats == nil || player[0].Participants[0].ReportedStats.Assists != 3 || player[0].Participants[0].SteamID != "" {
		t.Fatal("player stats or privacy projection incorrect")
	}
	history, err := a.matchHistory(ctx, steam, "")
	if err != nil || len(history.Matches) != 1 || history.Matches[0].ReportedT == nil || *history.Matches[0].ReportedT != 9 {
		t.Fatalf("history: %+v %v", history, err)
	}
	reply = call(report, token, 200)
	if reply.Servers[0].ResultAck == nil {
		t.Fatal("retry was not acknowledged")
	}
	rooms, err = a.matches(ctx)
	if err != nil || rooms[0].ResultRevision != 1 {
		t.Fatalf("retry changed revision: %+v %v", rooms, err)
	}
	conflict := *result
	conflict.T = 8
	bad.Servers[0].MatchResult = &conflict
	reply = call(bad, token, 200)
	if reply.Servers[0].ResultAck != nil {
		t.Fatal("conflicting result acknowledged")
	}
	conflict = *result
	conflict.Players = []protocol.ReportedPlayer{{SteamID: steam, Kills: 13, Deaths: 7, Assists: 3}}
	bad.Servers[0].MatchResult = &conflict
	if reply = call(bad, token, 200); reply.Servers[0].ResultAck != nil {
		t.Fatal("changed player stats acknowledged")
	}
	rooms, err = a.matches(ctx)
	if err != nil || *rooms[0].ReportedT != 9 || rooms[0].ResultRevision != 1 {
		t.Fatalf("conflict changed result: %+v %v", rooms, err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id) VALUES('server2','Server 2','localhost',27025,'','machine');
		INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id,load_fence_required,cancel_status) VALUES('CDEFGHIJKLMNOPQRSTUVWXYZ23','server2','Late','de_mirage','open','failed','BCDEFGHIJKLMNOPQRSTUVWXY23',true,'pending')`); err != nil {
		t.Fatal(err)
	}
	late := protocol.MachineReport{Driver: "managed", Servers: []protocol.ServerReport{{ID: "server2", Report: protocol.Report{Driver: "managed", State: "stopped", MatchResult: &protocol.MatchResult{SetupID: "BCDEFGHIJKLMNOPQRSTUVWXY23", CT: 13, T: 11}}}}}
	call(late, token, 200)
	var status, cancel, reason string
	if err := a.DB.QueryRow(ctx, `SELECT load_status,cancel_status,cancel_error FROM match_rooms WHERE id='CDEFGHIJKLMNOPQRSTUVWXYZ23'`).Scan(&status, &cancel, &reason); err != nil || status != "complete" || cancel != "failed" || reason == "" {
		t.Fatalf("late result did not supersede cancellation: %s %s %s %v", status, cancel, reason, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET reported_payload=NULL WHERE id='CDEFGHIJKLMNOPQRSTUVWXYZ23'`); err != nil {
		t.Fatal(err)
	}
	legacy := call(late, token, 200)
	var legacyAck bool
	for _, s := range legacy.Servers {
		if s.ID == "server2" && s.ResultAck != nil {
			legacyAck = true
		}
	}
	if !legacyAck {
		t.Fatal("legacy score-only retry not acknowledged")
	}
}
