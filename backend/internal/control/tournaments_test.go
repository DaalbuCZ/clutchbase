package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestTournamentPlayerProjection(t *testing.T) {
	team := &protocol.Team{ID: "team-a", CaptainSteamID: "player-a", Members: []protocol.TeamMember{{SteamID: "player-a"}}}
	base := []protocol.Tournament{{ID: "cup", Status: "open", PlayersPerSide: 1, Entries: []protocol.TournamentEntry{}}}
	base[0].Matches = []protocol.TournamentMatch{{Status: "pending", TeamA: &protocol.TournamentEntry{TeamID: "team-a"}, TeamB: &protocol.TournamentEntry{TeamID: "team-b"}, VetoTurnTeamID: "team-a", VetoBans: []string{}}}
	got := projectPlayerTournaments(base, "player-a", team)[0]
	if !got.CanRegisterTeam || got.CanCheckInTeam {
		t.Fatalf("captain should be able to register: %+v", got)
	}
	base[0].Status = "checkin"
	base[0].Entries = []protocol.TournamentEntry{{TeamID: team.ID, Status: "registered"}}
	got = projectPlayerTournaments(base, "player-a", team)[0]
	if !got.CanCheckInTeam || got.MyEntryStatus != "registered" {
		t.Fatalf("captain should be able to check in: %+v", got)
	}
	if got = projectPlayerTournaments(base, "player-b", nil)[0]; got.CanRegisterTeam || got.CanCheckInTeam {
		t.Fatalf("non-member was given an action: %+v", got)
	}
	base[0].Status = "active"
	if got = projectPlayerTournaments(base, "player-a", team)[0]; !got.Matches[0].CanVeto {
		t.Fatalf("player on the current veto team did not get the action: %+v", got.Matches[0])
	}
	if got = projectPlayerTournaments(base, "player-b", nil)[0]; got.Matches[0].CanVeto {
		t.Fatalf("player outside the team was given the veto action: %+v", got.Matches[0])
	}
}

func TestTeamTournamentFlow(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	players := []struct{ id, token string }{{"76561198154367261", strings.Repeat("a", 52)}, {"76561198154367262", strings.Repeat("b", 52)}, {"76561198154367263", strings.Repeat("c", 52)}}
	for _, p := range players {
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,$2)`, p.id, "Player"); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(p.token), p.id); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path string, body any, player int, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Origin", a.Origin)
		if player >= 0 {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: players[player].token})
		}
		if admin {
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	createTeam := func(name string, player int) string {
		w := call("POST", "/api/player/teams", map[string]string{"name": name}, player, false)
		if w.Code != 201 {
			t.Fatalf("create team: %d %s", w.Code, w.Body.String())
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.ID
	}
	one := createTeam("Alpha", 0)
	two := createTeam("Bravo", 1)
	invite := call("POST", "/api/player/teams/"+one+"/invite", map[string]any{}, 0, false)
	if invite.Code != 200 {
		t.Fatalf("create invite: %d %s", invite.Code, invite.Body.String())
	}
	var inviteResult struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(invite.Body.Bytes(), &inviteResult); err != nil || inviteResult.Code == "" {
		t.Fatal("invite code missing", err)
	}
	joined := call("POST", "/api/player/teams/join", map[string]string{"code": inviteResult.Code}, 2, false)
	if joined.Code != 204 {
		t.Fatalf("join team: %d %s", joined.Code, joined.Body.String())
	}
	created := call("POST", "/api/tournaments", map[string]any{"name": "Test cup", "playersPerSide": 1}, -1, true)
	if created.Code != 201 {
		t.Fatalf("create tournament: %d %s", created.Code, created.Body.String())
	}
	var tournament struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &tournament); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1} {
		w := call("POST", "/api/player/tournaments/"+tournament.ID+"/register", map[string]any{}, i, false)
		if w.Code != 204 {
			t.Fatalf("register team: %d %s", w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/tournaments/"+tournament.ID+"/checkin", map[string]any{}, -1, true); w.Code != 204 {
		t.Fatalf("start checkin: %d %s", w.Code, w.Body.String())
	}
	for _, i := range []int{0, 1} {
		w := call("POST", "/api/player/tournaments/"+tournament.ID+"/checkin", map[string]any{}, i, false)
		if w.Code != 204 {
			t.Fatalf("team checkin: %d %s", w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/tournaments/"+tournament.ID+"/bracket", map[string]any{}, -1, true); w.Code != 204 {
		t.Fatalf("build bracket: %d %s", w.Code, w.Body.String())
	}
	state, err := a.tournaments(ctx)
	if err != nil || len(state) != 1 || len(state[0].Matches) != 1 || state[0].Status != "active" {
		t.Fatalf("unexpected bracket: %+v %v", state, err)
	}
	fixture := state[0].Matches[0]
	teamAPlayer, teamBPlayer := 0, 1
	if fixture.TeamA.TeamID == two {
		teamAPlayer, teamBPlayer = 1, 0
	}
	if response := call("POST", "/api/player/tournaments/"+tournament.ID+"/matches/"+fixture.ID+"/veto", map[string]string{"map": state[0].MapPool[0]}, teamBPlayer, false); response.Code != 409 {
		t.Fatalf("out-of-turn veto was accepted: %d %s", response.Code, response.Body.String())
	}
	banOrder := []int{teamAPlayer, teamAPlayer, teamBPlayer, teamBPlayer, teamBPlayer, teamAPlayer}
	for i, player := range banOrder {
		response := call("POST", "/api/player/tournaments/"+tournament.ID+"/matches/"+fixture.ID+"/veto", map[string]string{"map": state[0].MapPool[i]}, player, false)
		if response.Code != 204 {
			t.Fatalf("map ban %d: %d %s", i+1, response.Code, response.Body.String())
		}
	}
	if response := call("POST", "/api/player/tournaments/"+tournament.ID+"/matches/"+fixture.ID+"/veto", map[string]string{"side": "ct"}, teamBPlayer, false); response.Code != 204 {
		t.Fatalf("team B starting side pick: %d %s", response.Code, response.Body.String())
	}
	state, err = a.tournaments(ctx)
	if err != nil || state[0].Matches[0].VetoStep != 7 || state[0].Matches[0].Map != state[0].MapPool[6] || state[0].Matches[0].TeamBSide != "ct" || !state[0].Matches[0].CanOpenRoom {
		t.Fatalf("veto result was not persisted: %+v %v", state, err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation) VALUES('tournament-machine','Tournament server','127.0.0.1','hash','managed',now(),true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,map,desired_state,observed_state,active_map,driver,last_seen,access_status,whitelist,preset,applied_preset,active_preset,machine_id,can_load_match) VALUES('tournament-host','Tournament server','127.0.0.1',27115,'hash','de_mirage','running','running','de_mirage','managed',now(),'verified',ARRAY[$1,$2]::text[],'match','match','match','tournament-machine',true)`, players[0].id, players[1].id); err != nil {
		t.Fatal(err)
	}
	fixtureID := state[0].Matches[0].ID
	room := call("POST", "/api/tournaments/"+tournament.ID+"/matches/"+fixtureID+"/room", map[string]string{"hostId": "tournament-host"}, -1, true)
	if room.Code != 201 {
		t.Fatalf("open fixture room: %d %s", room.Code, room.Body.String())
	}
	var roomResult struct {
		RoomID string `json:"roomId"`
	}
	if err := json.Unmarshal(room.Body.Bytes(), &roomResult); err != nil || roomResult.RoomID == "" {
		t.Fatal("fixture room id missing", err)
	}
	var ct, terrorist int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE side='ct'),count(*) FILTER(WHERE side='t') FROM match_room_players WHERE match_id=$1`, roomResult.RoomID).Scan(&ct, &terrorist); err != nil || ct != 1 || terrorist != 1 {
		t.Fatalf("team rosters were not assigned to sides: CT=%d T=%d err=%v", ct, terrorist, err)
	}
	if response := call("PATCH", "/api/player/matches/"+roomResult.RoomID+"/side", map[string]string{"side": "t"}, 0, false); response.Code != 409 {
		t.Fatal("tournament player changed their assigned side", response.Code, response.Body.String())
	}
	if response := call("POST", "/api/player/matches/"+roomResult.RoomID+"/leave", map[string]any{}, 0, false); response.Code != 409 {
		t.Fatal("tournament player left the fixture roster", response.Code, response.Body.String())
	}
	// Even a whitelisted player cannot take an empty tournament slot.
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=array_append(whitelist,$1) WHERE id='tournament-host'`, players[2].id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET players_per_side=2 WHERE id=$1`, roomResult.RoomID); err != nil {
		t.Fatal(err)
	}
	outsider, err := a.playerMatches(ctx, players[2].id)
	if err != nil || len(outsider) != 1 || outsider[0].CanJoin || outsider[0].Actions.JoinCT || outsider[0].Actions.JoinT {
		t.Fatalf("tournament exposed join actions to a whitelisted outsider: %+v %v", outsider, err)
	}
	member, err := a.playerMatches(ctx, players[0].id)
	if err != nil || len(member) != 1 || member[0].Actions.SwitchSide || member[0].Actions.Leave || !member[0].Actions.Ready {
		t.Fatalf("tournament roster actions are incorrect: %+v %v", member, err)
	}
	if response := call("POST", "/api/player/matches/"+roomResult.RoomID+"/join", map[string]string{"side": "ct"}, 2, false); response.Code != 409 {
		t.Fatalf("whitelisted outsider joined tournament fixture: %d %s", response.Code, response.Body.String())
	}
	if response := call("PATCH", "/api/player/matches/"+roomResult.RoomID+"/ready", map[string]bool{"ready": true}, 0, false); response.Code != 204 {
		t.Fatalf("fixture player could not ready: %d %s", response.Code, response.Body.String())
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET players_per_side=1 WHERE id=$1`, roomResult.RoomID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed',load_status='complete',result_ct=13,result_t=5 WHERE id=$1`, roomResult.RoomID); err != nil {
		t.Fatal(err)
	}
	state, err = a.tournaments(ctx)
	if err != nil || !state[0].Matches[0].CanReportWinner || state[0].Matches[0].RoomID != roomResult.RoomID || state[0].Matches[0].Map != state[0].MapPool[6] {
		t.Fatalf("fixture room was not projected to tournament: %+v %v", state, err)
	}
	winner := one
	if state[0].Matches[0].TeamB != nil && state[0].Matches[0].TeamB.TeamID == two {
		winner = two
	}
	matchID := state[0].Matches[0].ID
	reported := call("PATCH", "/api/tournaments/"+tournament.ID+"/matches/"+matchID+"/winner", map[string]string{"teamId": winner}, -1, true)
	if reported.Code != 204 {
		t.Fatalf("report winner: %d %s", reported.Code, reported.Body.String())
	}
	state, err = a.tournaments(ctx)
	if err != nil || state[0].Status != "complete" || state[0].WinnerTeamID != winner {
		t.Fatalf("winner was not persisted: %+v %v", state, err)
	}

	seriesCreated := call("POST", "/api/tournaments", map[string]any{"name": "Best of three", "playersPerSide": 1, "bestOf": 3}, -1, true)
	if seriesCreated.Code != 201 {
		t.Fatalf("create series tournament: %d %s", seriesCreated.Code, seriesCreated.Body.String())
	}
	var series struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(seriesCreated.Body.Bytes(), &series); err != nil {
		t.Fatal(err)
	}
	for i := range []string{one, two} {
		if response := call("POST", "/api/player/tournaments/"+series.ID+"/register", map[string]any{}, i, false); response.Code != 204 {
			t.Fatalf("register series team: %d %s", response.Code, response.Body.String())
		}
	}
	if response := call("POST", "/api/tournaments/"+series.ID+"/checkin", map[string]any{}, -1, true); response.Code != 204 {
		t.Fatalf("start series check-in: %d %s", response.Code, response.Body.String())
	}
	for _, i := range []int{0, 1} {
		if response := call("POST", "/api/player/tournaments/"+series.ID+"/checkin", map[string]any{}, i, false); response.Code != 204 {
			t.Fatalf("check in series team: %d %s", response.Code, response.Body.String())
		}
	}
	if response := call("POST", "/api/tournaments/"+series.ID+"/bracket", map[string]any{}, -1, true); response.Code != 204 {
		t.Fatalf("build series bracket: %d %s", response.Code, response.Body.String())
	}
	seriesState, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var seriesTournament protocol.Tournament
	for _, candidate := range seriesState {
		if candidate.ID == series.ID {
			seriesTournament = candidate
		}
	}
	seriesFixture := seriesTournament.Matches[0]
	if seriesTournament.BestOf != 3 {
		t.Fatalf("bestOf not stored: %+v", seriesTournament)
	}
	seriesTeamAPlayer, seriesTeamBPlayer := 0, 1
	if seriesFixture.TeamA.TeamID == two {
		seriesTeamAPlayer, seriesTeamBPlayer = 1, 0
	}
	steps := []struct {
		player  int
		mapName string
	}{
		{seriesTeamAPlayer, seriesTournament.MapPool[0]}, {seriesTeamBPlayer, seriesTournament.MapPool[1]},
		{seriesTeamAPlayer, seriesTournament.MapPool[2]}, {seriesTeamBPlayer, seriesTournament.MapPool[3]},
		{seriesTeamAPlayer, seriesTournament.MapPool[4]}, {seriesTeamBPlayer, seriesTournament.MapPool[5]},
	}
	for i, step := range steps {
		response := call("POST", "/api/player/tournaments/"+series.ID+"/matches/"+seriesFixture.ID+"/veto", map[string]string{"map": step.mapName}, step.player, false)
		if response.Code != 204 {
			t.Fatalf("series veto step %d: %d %s", i, response.Code, response.Body.String())
		}
	}
	if response := call("POST", "/api/player/tournaments/"+series.ID+"/matches/"+seriesFixture.ID+"/veto", map[string]string{"side": "t"}, seriesTeamBPlayer, false); response.Code != 204 {
		t.Fatalf("series side pick: %d %s", response.Code, response.Body.String())
	}
	seriesState, err = a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range seriesState {
		if candidate.ID == series.ID {
			seriesTournament = candidate
		}
	}
	wantMaps := []string{seriesTournament.MapPool[2], seriesTournament.MapPool[3], seriesTournament.MapPool[6]}
	if fmt.Sprint(seriesTournament.Matches[0].MapSequence) != fmt.Sprint(wantMaps) {
		t.Fatalf("wrong BO3 map order: got %v want %v", seriesTournament.Matches[0].MapSequence, wantMaps)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_map_observation) VALUES('tournament-series-machine','Tournament series server','127.0.0.2','hash','managed',now(),true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,map,desired_state,observed_state,active_map,driver,last_seen,access_status,whitelist,preset,applied_preset,active_preset,machine_id,can_load_match,supports_match_series,match_phase) VALUES('tournament-series-host','Tournament series server','127.0.0.1',27125,'hash','de_mirage','running','running','de_mirage','managed',now(),'verified',ARRAY[$1,$2]::text[],'match','match','match','tournament-series-machine',true,true,'idle')`, players[0].id, players[1].id); err != nil {
		t.Fatal(err)
	}
	seriesRoom := call("POST", "/api/tournaments/"+series.ID+"/matches/"+seriesFixture.ID+"/room", map[string]string{"hostId": "tournament-series-host"}, -1, true)
	if seriesRoom.Code != 201 {
		t.Fatalf("open BO3 room: %d %s", seriesRoom.Code, seriesRoom.Body.String())
	}
	var seriesRoomResult struct {
		RoomID string `json:"roomId"`
	}
	if err := json.Unmarshal(seriesRoom.Body.Bytes(), &seriesRoomResult); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_room_players SET ready=true WHERE match_id=$1`, seriesRoomResult.RoomID); err != nil {
		t.Fatal(err)
	}
	load := call("POST", "/api/matches/"+seriesRoomResult.RoomID+"/load", map[string]any{}, -1, true)
	if load.Code != 202 {
		t.Fatalf("load BO3 setup: %d %s", load.Code, load.Body.String())
	}
	var setup protocol.MatchSetup
	var setupJSON []byte
	if err := a.DB.QueryRow(ctx, `SELECT load_payload FROM match_rooms WHERE id=$1`, seriesRoomResult.RoomID).Scan(&setupJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(setupJSON, &setup); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(setup.Maps) != fmt.Sprint(wantMaps) || fmt.Sprint(setup.MapSides) != fmt.Sprint([]string{"team1_ct", "team1_t", "team1_ct"}) {
		t.Fatalf("wrong MatchZy series payload: %+v", setup)
	}
}

func TestTournamentBracketByes(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	for _, entrants := range []int{3, 5} {
		t.Run(fmt.Sprintf("%d entrants", entrants), func(t *testing.T) {
			tournamentID := rand.Text()
			if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status) VALUES($1,$2,1,'checkin')`, tournamentID, fmt.Sprintf("Cup %d", entrants)); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < entrants; i++ {
				steam := fmt.Sprintf("76561198154%06d", entrants*100+i)
				teamID := rand.Text()
				if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,$2)`, steam, "Player"); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$2,$3)`, teamID, fmt.Sprintf("Team %d %d", entrants, i), steam); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO team_members(team_id,steam_id) VALUES($1,$2)`, teamID, steam); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'checked_in')`, tournamentID, teamID); err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest(http.MethodPost, "/api/tournaments/"+tournamentID+"/bracket", bytes.NewReader([]byte(`{}`)))
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, req)
			if w.Code != 204 {
				t.Fatalf("build bracket: %d %s", w.Code, w.Body.String())
			}
			var matches, invalid int
			if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM tournament_matches WHERE tournament_id=$1`, tournamentID).Scan(&matches); err != nil {
				t.Fatal(err)
			}
			if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM tournament_matches m WHERE m.tournament_id=$1 AND m.status='pending' AND m.round>1 AND ((m.team_a IS NULL AND EXISTS(SELECT 1 FROM tournament_matches p WHERE p.tournament_id=m.tournament_id AND p.round=m.round-1 AND p.number=m.number*2-1 AND p.status<>'pending')) OR (m.team_b IS NULL AND EXISTS(SELECT 1 FROM tournament_matches p WHERE p.tournament_id=m.tournament_id AND p.round=m.round-1 AND p.number=m.number*2 AND p.status<>'pending')))`, tournamentID).Scan(&invalid); err != nil {
				t.Fatal(err)
			}
			size := 1
			for size < entrants {
				size *= 2
			}
			if matches != size-1 || invalid != 0 {
				rows, _ := a.DB.Query(ctx, `SELECT round,number,status,team_a,team_b,winner_team_id FROM tournament_matches WHERE tournament_id=$1 ORDER BY round,number`, tournamentID)
				defer rows.Close()
				for rows.Next() {
					var round, number int
					var status string
					var aa, bb, win *string
					_ = rows.Scan(&round, &number, &status, &aa, &bb, &win)
					t.Logf("round %d match %d status %s teams %v/%v winner %v", round, number, status, aa, bb, win)
				}
				t.Fatalf("bad %d-team bracket: matches=%d unresolved byes=%d", entrants, matches, invalid)
			}
		})
	}
}
