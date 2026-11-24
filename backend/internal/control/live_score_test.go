package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
)

func TestLiveScoreIsBoundToLoadedRoomAndFreshRuntime(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const other = "BCDEFGHIJKLMNOPQRSTUVWXY23"
	const room = "CDEFGHIJKLMNOPQRSTUVWXYZ23"
	const steam = "76561198154367261"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO machines(id,name,address,token_hash,supports_match_control) VALUES('machine','Machine','localhost',$1,true)`, digest("agent-token"))
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,whitelist) VALUES('host','Host','localhost',27015,'','machine','running',ARRAY[$1]::text[])`, steam)
	exec(`INSERT INTO players(steam_id) VALUES($1)`, steam)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id) VALUES($1,'host','Room','de_mirage','open','complete',$2)`, room, setup)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,'ct')`, room, steam)
	report := protocol.MachineReport{Driver: "managed", SupportsMatchControl: true, SupportsMapObservation: true, Servers: []protocol.ServerReport{{ID: "host", Report: protocol.Report{Driver: "managed", State: "running", AccessStatus: "verified", ActivePreset: "match", MatchPhase: "live", ActiveMap: "de_mirage", ActiveMatchSetupID: setup, LiveScore: &protocol.LiveScore{SetupID: setup, Map: "de_mirage", CT: 8, T: 5, ObservedAt: time.Now().UnixMilli()}}}}}
	call := func(want int) {
		t.Helper()
		data, _ := json.Marshal(report)
		r := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", strings.NewReader(string(data)))
		r.Header.Set("Authorization", "Bearer agent-token")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("heartbeat %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	score := func() *protocol.LiveScore {
		t.Helper()
		matches, err := a.matches(ctx)
		if err != nil || len(matches) != 1 {
			t.Fatalf("admin match snapshot: %+v %v", matches, err)
		}
		public, err := a.playerMatches(ctx, steam)
		if err != nil || len(public) != 1 {
			t.Fatalf("public snapshot: %+v %v", public, err)
		}
		if (matches[0].LiveScore == nil) != (public[0].LiveScore == nil) {
			t.Fatal("admin/player score policy differs")
		}
		return matches[0].LiveScore
	}
	call(200)
	if s := score(); s == nil || s.CT != 8 || s.T != 5 {
		t.Fatalf("missing score: %+v", s)
	}
	hidden, err := a.playerMatches(ctx, "76561198154367262")
	if err != nil || len(hidden) != 0 {
		t.Fatal("score exposed to player without server access")
	}
	report.Servers[0].LiveScore.CT = 100
	call(400)
	report.Servers[0].LiveScore.CT = 8
	report.Servers[0].LiveScore.SetupID = other
	call(400)
	report.Servers[0].LiveScore.SetupID = setup
	report.Servers[0].LiveScore.Map = "de_nuke"
	call(400)
	report.Servers[0].LiveScore.Map = "de_mirage"
	report.Servers[0].MatchPhase = "paused"
	call(200)
	if score() == nil {
		t.Fatal("paused score missing")
	}
	report.Servers[0].LiveScore.ObservedAt = time.Now().Add(-16 * time.Second).UnixMilli()
	call(200)
	if score() != nil {
		t.Fatal("old observation survived a fresh heartbeat")
	}
	report.Servers[0].LiveScore.ObservedAt = time.Now().Add(10 * time.Second).UnixMilli()
	call(200)
	if score() != nil {
		t.Fatal("future observation accepted")
	}
	report.Servers[0].LiveScore.ObservedAt = time.Now().UnixMilli()
	call(200)
	exec(`UPDATE hosts SET last_seen=now()-interval '30 seconds' WHERE id='host'`)
	if score() != nil {
		t.Fatal("offline host exposed live score")
	}
	call(200)
	exec(`UPDATE hosts SET live_score=jsonb_set(live_score,'{observedAt}',to_jsonb((extract(epoch FROM now()-interval '16 seconds')*1000)::bigint)) WHERE id='host'`)
	if score() != nil {
		t.Fatal("query did not expire stored score")
	}
	call(200)
	report.Servers[0].ActiveMatchSetupID = other
	report.Servers[0].LiveScore.SetupID = other
	call(200)
	if score() != nil {
		t.Fatal("old room displayed another setup's score")
	}
	report.Servers[0].ActiveMatchSetupID = setup
	report.Servers[0].LiveScore.SetupID = setup
	call(200)
	report.Servers[0].LiveScore = nil
	call(200)
	if score() != nil {
		t.Fatal("old reporter retained an earlier score")
	}
}

func TestLiveScoreExpiresOnOpenWebsocket(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const setup = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	observed := time.Now().Add(-13 * time.Second).UnixMilli()
	payload, _ := json.Marshal(protocol.LiveScore{SetupID: setup, Map: "de_mirage", CT: 0, T: 0, ObservedAt: observed})
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,desired_state,observed_state,last_seen,driver,access_status,active_preset,active_map,active_match_setup_id,match_phase,live_score) VALUES('host','Host','localhost',27015,'','running','running',now(),'managed','verified','match','de_mirage',$1,'live',$2::jsonb)`, setup, string(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','host','Room','de_mirage','open','complete',$1)`, setup); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live", a.AdminToken, http.Header{"Origin": {a.Origin}})
	defer c.CloseNow()
	var state protocol.Workspace
	readLive(t, c, &state)
	if len(state.Matches) != 1 || state.Matches[0].LiveScore == nil {
		t.Fatal("initial live 0-0 score missing")
	}
	readLive(t, c, &state)
	if state.Matches[0].LiveScore != nil {
		t.Fatal("live score did not expire without database mutation")
	}
}
