package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayerRoomActionsAndRosterReadiness(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	const first = "76561198154367261"
	const second = "76561198154367262"
	const third = "76561198154367263"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,desired_state,observed_state,driver,last_seen,access_status,active_preset,whitelist)
		VALUES('host','Host','localhost',27015,'','running','running','managed',now(),'verified','match',ARRAY['76561198154367261','76561198154367262','76561198154367263']);
		INSERT INTO match_rooms(id,host_id,title,map,players_per_side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friends','de_mirage',3);
		INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'),('76561198154367263');
		INSERT INTO match_room_players(match_id,steam_id,side,ready) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct',true),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t',false)`); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{first: strings.Repeat("a", 52), second: strings.Repeat("b", 52), third: strings.Repeat("c", 52)}
	for steam, token := range tokens {
		if _, err := a.DB.Exec(ctx, "INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')", digest(token), steam); err != nil {
			t.Fatal(err)
		}
	}
	call := func(steam, method, action, body string) {
		t.Helper()
		req := httptest.NewRequest(method, "/api/player/matches/"+room+"/"+action, strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[steam]})
		req.Header.Set("Origin", a.Origin)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != http.StatusNoContent {
			t.Fatalf("%s %s: %d %s", method, action, res.Code, res.Body.String())
		}
	}
	view := func(steam string) playerMatch {
		t.Helper()
		matches, err := a.playerMatches(ctx, steam)
		if err != nil || len(matches) != 1 {
			t.Fatalf("room snapshot: %+v %v", matches, err)
		}
		return matches[0]
	}
	outsider := view(third)
	if outsider.Joined || !outsider.Actions.JoinCT || !outsider.Actions.JoinT || outsider.Actions.Ready || outsider.Actions.Leave {
		t.Fatalf("incorrect join actions: %+v", outsider.Actions)
	}
	call(third, "POST", "join", `{"side":"ct"}`)
	if firstView := view(first); firstView.MyReady || !firstView.Actions.Ready || firstView.Actions.Unready {
		t.Fatalf("join did not clear readiness: %+v", firstView)
	}
	call(first, "PATCH", "ready", `{"ready":true}`)
	call(third, "POST", "join", `{"side":"ct"}`)
	if firstView := view(first); !firstView.MyReady || !firstView.Actions.Unready || firstView.Actions.SwitchSide {
		t.Fatalf("replayed join changed readiness: %+v", firstView)
	}
	call(second, "PATCH", "side", `{"side":"ct"}`)
	if firstView := view(first); firstView.MyReady {
		t.Fatalf("side change did not clear readiness: %+v", firstView)
	}
	call(first, "PATCH", "ready", `{"ready":true}`)
	call(second, "PATCH", "side", `{"side":"ct"}`)
	if firstView := view(first); !firstView.MyReady {
		t.Fatalf("unchanged side cleared readiness: %+v", firstView)
	}
	call(third, "POST", "leave", "")
	if firstView := view(first); firstView.MyReady {
		t.Fatalf("leaving did not clear readiness: %+v", firstView)
	}
	call(first, "PATCH", "ready", `{"ready":true}`)
	call(third, "POST", "leave", "")
	if firstView := view(first); !firstView.MyReady {
		t.Fatalf("replayed leave cleared readiness: %+v", firstView)
	}
	if _, err := a.DB.Exec(ctx, "UPDATE match_rooms SET load_status='pending' WHERE id=$1", room); err != nil {
		t.Fatal(err)
	}
	locked := view(first)
	if locked.Actions != (playerMatchActions{Report: true}) {
		t.Fatalf("pending load exposed actions: %+v", locked.Actions)
	}
}
