package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayerMatchControlRequiresRosterAndCurrentSetup(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const setup = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	const member = "76561198154367261"
	const outsider = "76561198154367262"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id) VALUES($1),($2)`, member, outsider)
	for i, id := range []string{member, outsider} {
		exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(strings.Repeat(string(rune('a'+i)), 52)), id)
	}
	exec(`INSERT INTO machines(id,name,address,token_hash,last_seen,supports_match_control) VALUES('machine','Machine','localhost','',now(),true)`)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen,access_status,active_preset,match_phase,can_pause_match,active_match_setup_id,whitelist,revision) VALUES('host','Host','localhost',27015,'','machine','running','running','managed',now(),'verified','match','live',true,$1,ARRAY[$2,$3]::text[],1)`, setup, member, outsider)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id) VALUES($1,'host','Friends','de_mirage','open','complete',$2)`, room, setup)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,'ct')`, room, member)
	call := func(cookie, origin, body string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/player/matches/"+room+"/control", strings.NewReader(body))
		r.Header.Set("Origin", origin)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: playerCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", body, w.Code, want, w.Body.String())
		}
	}
	view := func(id string) playerMatch {
		t.Helper()
		ms, err := a.playerMatches(ctx, id)
		if err != nil || len(ms) != 1 {
			t.Fatalf("snapshot: %+v %v", ms, err)
		}
		return ms[0]
	}
	token := strings.Repeat("a", 52)
	if m := view(member); !m.Actions.Pause || m.Actions.Resume || m.ControlRevision != 1 || m.MatchPhase != "live" {
		t.Fatalf("bad live member snapshot: %+v", m)
	}
	if m := view(outsider); m.Actions.Pause || m.Actions.Resume {
		t.Fatal("nonparticipant got control actions")
	}
	call("", a.Origin, `{"action":"pause","revision":1}`, 401)
	call(token, "https://other.example", `{"action":"pause","revision":1}`, 403)
	call(strings.Repeat("b", 52), a.Origin, `{"action":"pause","revision":1}`, 404)
	call(token, a.Origin, `{"action":"end","revision":1}`, 400)
	call(token, a.Origin, `{"action":"pause"}`, 409)
	call(token, a.Origin, `{"action":"pause","revision":0}`, 409)
	call(token, a.Origin, `{"action":"resume","revision":1}`, 409)
	exec(`UPDATE hosts SET active_match_setup_id='CDEFGHIJKLMNOPQRSTUVWXYZ23' WHERE id='host'`)
	if view(member).Actions.Pause {
		t.Fatal("old room controls another setup")
	}
	call(token, a.Origin, `{"action":"pause","revision":1}`, 409)
	exec(`UPDATE hosts SET active_match_setup_id=$1,last_seen=now()-interval '1 minute' WHERE id='host'`, setup)
	if m := view(member); m.Actions.Pause || m.MatchPhase != "unknown" {
		t.Fatal("stale host offered controls or live phase")
	}
	call(token, a.Origin, `{"action":"pause","revision":1}`, 409)
	exec(`UPDATE hosts SET last_seen=NULL WHERE id='host'`)
	if view(member).Actions.Pause {
		t.Fatal("missing heartbeat offered controls")
	}
	exec(`UPDATE hosts SET last_seen=now(),can_pause_match=false WHERE id='host'`)
	call(token, a.Origin, `{"action":"pause","revision":1}`, 409)
	exec(`UPDATE hosts SET can_pause_match=true WHERE id='host'`)
	call(token, a.Origin, `{"action":"pause","revision":1}`, 202)
	m := view(member)
	if m.ControlStatus != "pending" || m.MatchPhase != "live" || m.Actions.Pause || m.Actions.Resume {
		t.Fatalf("request changed lifecycle optimistically: %+v", m)
	}
	var commandSetup string
	if err := a.DB.QueryRow(ctx, `SELECT match_control_setup_id FROM hosts WHERE id='host'`).Scan(&commandSetup); err != nil || commandSetup != setup {
		t.Fatalf("unfenced request: %s %v", commandSetup, err)
	}
	call(token, a.Origin, `{"action":"pause","revision":2}`, 409)
	exec(`UPDATE hosts SET match_phase='paused',match_control_status='complete',can_pause_match=false,can_resume_match=true WHERE id='host'`)
	m = view(member)
	if !m.Actions.Resume || m.Actions.Pause || m.MatchPhase != "paused" {
		t.Fatalf("bad resume snapshot: %+v", m)
	}
	call(token, a.Origin, `{"action":"resume","revision":2}`, 202)
	exec(`UPDATE hosts SET match_control_status='complete',whitelist=ARRAY[$1]::text[] WHERE id='host'`, outsider)
	call(token, a.Origin, `{"action":"resume","revision":3}`, 404)
	exec(`UPDATE hosts SET whitelist=ARRAY[$1,$2]::text[] WHERE id='host'`, member, outsider)
	exec(`UPDATE match_rooms SET status='closed' WHERE id=$1`, room)
	if view(member).Actions.Resume {
		t.Fatal("closed room offered controls")
	}
	call(token, a.Origin, `{"action":"resume","revision":3}`, 409)
	// The public payload contains no private host or agent credential fields.
	data, err := json.Marshal(view(member))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "token") || strings.Contains(string(data), "password") {
		t.Fatal("private host fields exposed")
	}
}
