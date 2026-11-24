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

func TestTournamentWithdrawalPermissionsAndRosterUnlock(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const captain = "76561198154367261"
	const member = "76561198154367262"
	tokens := []string{strings.Repeat("a", 52), strings.Repeat("b", 52)}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{captain, member} {
		exec(`INSERT INTO players(steam_id) VALUES($1)`, id)
		exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(tokens[i]), id)
	}
	exec(`INSERT INTO teams(id,name,captain_steam_id) VALUES('team','Friends',$1)`, captain)
	exec(`INSERT INTO team_members(team_id,steam_id) VALUES('team',$1),('team',$2)`, captain, member)
	exec(`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'open')`)
	exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','team','registered')`)
	call := func(method, path string, body any, player int, admin bool, want int) {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Origin", a.Origin)
		if player >= 0 {
			r.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[player]})
		}
		if admin {
			r.Header.Set("Authorization", "Bearer "+a.AdminToken)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
	}
	selection := func(phase, status string) map[string]any {
		var revision int64
		if err := a.DB.QueryRow(ctx, `SELECT revision FROM teams WHERE id='team'`).Scan(&revision); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"teamId": "team", "entryStatus": status, "phase": phase, "teamRevision": revision}
	}
	path := "/api/player/tournaments/cup/registration"
	call("DELETE", path, selection("open", "registered"), -1, false, 401)
	call("DELETE", path, selection("open", "registered"), 1, false, 404)
	call("DELETE", path, selection("checkin", "registered"), 0, false, 409)
	call("DELETE", path, selection("open", "checked_in"), 0, false, 409)
	ts, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	team, err := a.playerTeam(ctx, captain)
	if err != nil {
		t.Fatal(err)
	}
	if p := projectPlayerTournaments(ts, captain, team)[0]; !p.CanWithdrawTeam {
		t.Fatal("captain cannot withdraw registration")
	}
	if p := projectPlayerTournaments(ts, member, team)[0]; p.CanWithdrawTeam {
		t.Fatal("member can withdraw team")
	}
	stale := selection("open", "registered")
	stale["teamRevision"] = 99
	call("DELETE", path, stale, 0, false, 409)
	oldRegistration := selection("open", "registered")
	call("DELETE", path, selection("open", "registered"), 0, false, 204)
	team, err = a.playerTeam(ctx, captain)
	if err != nil || team.Registered || !team.Actions.Disband || team.Revision != 1 {
		t.Fatalf("withdrawal didn't update team snapshot: %+v %v", team, err)
	}
	call("POST", "/api/player/tournaments/cup/register", nil, 0, false, 204)
	call("DELETE", path, oldRegistration, 0, false, 409)
	call("POST", "/api/tournaments/cup/checkin", nil, -1, true, 204)
	call("POST", "/api/player/tournaments/cup/checkin", nil, 0, false, 204)
	team, err = a.playerTeam(ctx, captain)
	if err != nil || !team.RosterLocked {
		t.Fatalf("check-in didn't lock: %+v %v", team, err)
	}
	// Another checked-in registration must keep its independent roster lock.
	exec(`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('other','Other cup',1,'checkin')`)
	exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('other','team','checked_in')`)
	call("DELETE", path, selection("checkin", "checked_in"), 0, false, 204)
	team, err = a.playerTeam(ctx, captain)
	if err != nil || !team.RosterLocked {
		t.Fatalf("withdrawing one cup unlocked another: %+v %v", team, err)
	}
	call("DELETE", "/api/player/tournaments/other/registration", selection("checkin", "checked_in"), 0, false, 204)
	team, err = a.playerTeam(ctx, captain)
	if err != nil || team.RosterLocked || team.Registered || !team.Actions.Disband {
		t.Fatalf("last withdrawal didn't release lock: %+v %v", team, err)
	}
	call("POST", "/api/player/tournaments/cup/register", nil, 0, false, 409)
}

func TestWithdrawalAndFixtureGenerationSerialize(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := strings.Repeat("w", 52)
	for i, id := range []string{"76561198154367261", "76561198154367262", "76561198154367263"} {
		team := string(rune('a' + i))
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, team, id); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO team_members(team_id,steam_id) VALUES($1,$2)`, team, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,'76561198154367261',now()+interval '1 hour');`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'checkin'); INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','checked_in'),('cup','c','checked_in')`); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	withdrawn := make(chan int, 1)
	generated := make(chan int, 1)
	go func() {
		<-start
		r := httptest.NewRequest("DELETE", "/api/player/tournaments/cup/registration", strings.NewReader(`{"teamId":"a","entryStatus":"checked_in","phase":"checkin","teamRevision":0}`))
		r.Header.Set("Origin", a.Origin)
		r.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		withdrawn <- w.Code
	}()
	go func() {
		<-start
		r := httptest.NewRequest("POST", "/api/tournaments/cup/bracket", nil)
		r.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		generated <- w.Code
	}()
	close(start)
	withdrawCode, generateCode := <-withdrawn, <-generated
	if generateCode != 204 || (withdrawCode != 204 && withdrawCode != 409) {
		t.Fatalf("withdraw=%d generate=%d", withdrawCode, generateCode)
	}
	var entry, fixture bool
	if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tournament_entries WHERE tournament_id='cup' AND team_id='a'),EXISTS(SELECT 1 FROM tournament_matches WHERE tournament_id='cup' AND (team_a='a' OR team_b='a'))`).Scan(&entry, &fixture); err != nil {
		t.Fatal(err)
	}
	if entry != fixture || entry != (withdrawCode == 409) {
		t.Fatalf("withdrawal corrupted fixture ownership: entry=%v fixture=%v response=%d", entry, fixture, withdrawCode)
	}
}
