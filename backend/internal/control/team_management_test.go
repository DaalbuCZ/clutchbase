package control

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamManagementPermissionsLocksAndArchive(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	players := []string{"76561198154367261", "76561198154367262", "76561198154367263"}
	tokens := []string{strings.Repeat("a", 52), strings.Repeat("b", 52), strings.Repeat("c", 52)}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range players {
		exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Friend')`, id)
		exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(tokens[i]), id)
	}
	exec(`INSERT INTO teams(id,name,captain_steam_id) VALUES('team','Friends',$1)`, players[0])
	exec(`INSERT INTO team_members(team_id,steam_id) VALUES('team',$1),('team',$2)`, players[0], players[1])
	call := func(method, path string, body any, player int, want int) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Origin", a.Origin)
		if player >= 0 {
			r.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[player]})
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	rev := func(n int) map[string]any { return map[string]any{"revision": n} }
	target := "/api/player/teams/team/members/" + players[1]
	call("DELETE", target, rev(0), -1, 401)
	call("DELETE", target, rev(0), 1, 404)
	call("DELETE", "/api/player/teams/team/members/"+players[0], rev(0), 0, 409)
	call("DELETE", target, rev(9), 0, 409)
	invite := call("POST", "/api/player/teams/team/invite", nil, 0, 200)
	var code struct{ Code string }
	if err := json.Unmarshal(invite.Body.Bytes(), &code); err != nil {
		t.Fatal(err)
	}
	call("PATCH", "/api/player/teams/team/captain", map[string]any{"revision": 0, "steamId": players[1]}, 0, 204)
	call("POST", "/api/player/teams/join", map[string]string{"code": code.Code}, 2, 404)
	call("DELETE", target, rev(1), 0, 404)
	team, err := a.playerTeam(ctx, players[1])
	if err != nil || team == nil || team.Revision != 1 || !team.Actions.RemoveMember || !team.Actions.TransferCaptain || !team.Actions.Disband {
		t.Fatalf("captain snapshot: %+v %v", team, err)
	}
	call("DELETE", "/api/player/teams/team/members/"+players[0], rev(1), 1, 204)
	team, err = a.playerTeam(ctx, players[0])
	if err != nil || team != nil {
		t.Fatalf("removed member still on team: %+v %v", team, err)
	}
	// An invite generated before check-in must also obey the current roster lock.
	invite = call("POST", "/api/player/teams/team/invite", nil, 1, 200)
	if err = json.Unmarshal(invite.Body.Bytes(), &code); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO team_members(team_id,steam_id) VALUES('team',$1)`, players[2])
	exec(`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'checkin')`)
	exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','team','checked_in')`)
	team, err = a.playerTeam(ctx, players[1])
	if err != nil || !team.RosterLocked || team.Actions.Invite || team.Actions.Disband || team.Actions.RemoveMember || team.Actions.TransferCaptain {
		t.Fatalf("checked-in captain actions: %+v %v", team, err)
	}
	call("POST", "/api/player/teams/team/invite", nil, 1, 409)
	call("POST", "/api/player/teams/join", map[string]string{"code": code.Code}, 0, 409)
	call("POST", "/api/player/teams/leave", nil, 2, 409)
	call("DELETE", "/api/player/teams/team/members/"+players[2], rev(2), 1, 409)
	call("PATCH", "/api/player/teams/team/captain", map[string]any{"revision": 2, "steamId": players[2]}, 1, 409)
	call("DELETE", "/api/player/teams/team", rev(2), 1, 409)
	exec(`UPDATE tournament_entries SET status='registered' WHERE tournament_id='cup'`)
	call("DELETE", "/api/player/teams/team", rev(2), 1, 409)
	exec(`UPDATE tournaments SET status='complete',winner_team_id='team' WHERE id='cup'`)
	exec(`INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,winner_team_id,status) VALUES('fixture','cup',1,1,'team','team','bye')`)
	call("DELETE", "/api/player/teams/team", rev(2), 1, 204)
	for _, id := range players {
		team, err = a.playerTeam(ctx, id)
		if err != nil || team != nil {
			t.Fatalf("archived membership remained: %+v %v", team, err)
		}
	}
	var name string
	if err = a.DB.QueryRow(ctx, `SELECT t.name FROM tournament_matches m JOIN teams t ON t.id=m.winner_team_id WHERE m.id='fixture'`).Scan(&name); err != nil || name != "Friends" {
		t.Fatalf("historical team lost: %s %v", name, err)
	}
	call("POST", "/api/player/teams/team/invite", nil, 1, 404)
	call("POST", "/api/player/teams", map[string]string{"name": "New team"}, 1, 201)
}

func TestConcurrentTeamEditsUseOneRevision(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	ids := []string{"76561198154367261", "76561198154367262", "76561198154367263"}
	for _, id := range ids {
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	token := strings.Repeat("r", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES('team','Friends',$1)`, ids[0]); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, err := a.DB.Exec(ctx, `INSERT INTO team_members(team_id,steam_id) VALUES('team',$1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, id := range ids[1:] {
		go func(id string) {
			<-start
			r := httptest.NewRequest("DELETE", "/api/player/teams/team/members/"+id, strings.NewReader(`{"revision":0}`))
			r.Header.Set("Origin", a.Origin)
			r.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			results <- w.Code
		}(id)
	}
	close(start)
	counts := map[int]int{}
	for range 2 {
		counts[<-results]++
	}
	if counts[204] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent roster edits bypassed revision: %+v", counts)
	}
	team, err := a.playerTeam(ctx, ids[0])
	if err != nil || team.Revision != 1 || len(team.Members) != 2 {
		t.Fatalf("bad final roster: %+v %v", team, err)
	}
}
