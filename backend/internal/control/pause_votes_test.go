package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPauseVotesThresholdAndInvalidation(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const setup = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	const one = "76561198154367261"
	const two = "76561198154367262"
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id) VALUES($1),($2)`, one, two)
	exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour'),($3,$4,now()+interval '1 hour')`, digest(strings.Repeat("a", 52)), one, digest(strings.Repeat("b", 52)), two)
	exec(`INSERT INTO machines(id,name,address,token_hash,last_seen,supports_match_control) VALUES('machine','Machine','localhost','',now(),true)`)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen,access_status,active_preset,match_phase,can_pause_match,active_match_setup_id,whitelist,revision) VALUES('host','Host','localhost',27015,'','machine','running','running','managed',now(),'verified','match','live',true,$1,ARRAY[$2,$3]::text[],1)`, setup, one, two)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id,pause_vote_threshold) VALUES($1,'host','Friends','de_mirage','open','complete',$2,2)`, room, setup)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,'ct'),($1,$3,'t')`, room, one, two)
	call := func(token, body string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/player/matches/"+room+"/control", strings.NewReader(body))
		r.Header.Set("Origin", a.Origin)
		r.AddCookie(&http.Cookie{Name: playerCookie, Value: strings.Repeat(token, 52)})
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	call("a", `{"action":"pause","revision":1}`, 202)
	ms, err := a.playerMatches(ctx, one)
	if err != nil || len(ms) != 1 || ms[0].PauseVotes != 1 || !ms[0].MyPauseVote || ms[0].Actions.Pause || ms[0].ControlStatus == "pending" {
		t.Fatalf("first vote: %+v %v", ms, err)
	}
	call("b", `{"action":"pause","revision":1}`, 409)
	call("a", `{"action":"pause","revision":2}`, 409)
	call("b", `{"action":"pause","revision":2}`, 202)
	ms, err = a.playerMatches(ctx, two)
	if err != nil || ms[0].ControlStatus != "pending" || ms[0].PauseVotes != 0 || ms[0].MatchPhase != "live" {
		t.Fatalf("threshold: %+v %v", ms, err)
	}
	exec(`UPDATE hosts SET match_control_status='complete',match_phase='paused',can_pause_match=false,can_resume_match=true WHERE id='host'`)
	exec(`UPDATE hosts SET match_phase='live',can_pause_match=true,can_resume_match=false WHERE id='host'`)
	call("a", `{"action":"pause","revision":3}`, 202)
	exec(`UPDATE hosts SET active_match_setup_id='CDEFGHIJKLMNOPQRSTUVWXYZ23' WHERE id='host'`)
	var count int
	if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM match_pause_votes`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("votes retained: %d %v", count, err)
	}
}

func TestPauseVotingConfiguration(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,revision) VALUES('host','Host','localhost',27015,'',1); INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage')`); err != nil {
		t.Fatal(err)
	}
	call := func(body string, want int) {
		t.Helper()
		r := httptest.NewRequest("PATCH", "/api/matches/ABCDEFGHIJKLMNOPQRSTUVWX23/pause-voting", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	call(`{"threshold":0,"revision":1}`, 400)
	call(`{"threshold":2,"revision":0}`, 409)
	call(`{"threshold":2,"revision":1}`, 200)
	matches, err := a.matches(ctx)
	if err != nil || len(matches) != 1 || matches[0].PauseVoteThreshold != 2 || matches[0].ControlRevision != 2 || !matches[0].CanConfigurePauseVoting {
		t.Fatalf("snapshot: %+v %v", matches, err)
	}
	call(`{"threshold":3,"revision":1}`, 409)
}
