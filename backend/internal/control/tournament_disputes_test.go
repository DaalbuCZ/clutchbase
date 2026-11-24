package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTournamentDisputePrivacyAndRevision(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const player = "76561198154367261"
	const outsider = "76561198154367262"
	token := strings.Repeat("a", 52)
	other := strings.Repeat("b", 52)
	for _, q := range []string{
		`INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262')`,
		`INSERT INTO hosts(id,name,address,port,token_hash) VALUES('host','Host','localhost',27015,'')`,
		`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,reported_ct,reported_t,reported_at) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage','closed','complete',13,9,now())`,
		`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct')`,
		`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'complete')`,
		`INSERT INTO tournament_matches(id,tournament_id,round,number,status,room_id) VALUES('fixture','cup',1,1,'complete','ABCDEFGHIJKLMNOPQRSTUVWX23')`,
	} {
		if _, err := a.DB.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	for steam, secret := range map[string]string{player: token, outsider: other} {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(secret), steam); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path, body, secret string, admin bool) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", a.Origin)
		if admin {
			r.Header.Set("Authorization", "Bearer "+secret)
		} else if secret != "" {
			r.AddCookie(&http.Cookie{Name: playerCookie, Value: secret})
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w.Code
	}
	path := "/api/player/tournaments/cup/matches/fixture/dispute"
	if got := call("POST", path, `{"reason":"Wrong"}`, other, false); got != 403 {
		t.Fatal("outsider", got)
	}
	before, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !projectPlayerTournaments(before, player, nil)[0].Matches[0].CanDispute {
		t.Fatal("missing player action")
	}
	if got := call("POST", path, `{"reason":" Wrong score "}`, token, false); got != 204 {
		t.Fatal("submit", got)
	}
	if got := call("POST", path, `{"reason":"Again"}`, token, false); got != 409 {
		t.Fatal("duplicate", got)
	}
	state, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if projectPlayerTournaments(state, outsider, nil)[0].Matches[0].Dispute != nil {
		t.Fatal("dispute exposed to outsider")
	}
	if d := projectPlayerTournaments(state, player, nil)[0].Matches[0].Dispute; d == nil || d.Reason != "Wrong score" {
		t.Fatalf("participant dispute %+v", d)
	}
	review := "/api/tournaments/cup/matches/fixture/dispute"
	if got := call("PATCH", review, `{"revision":1,"resolution":"Confirmed"}`, a.AdminToken, true); got != 409 {
		t.Fatal("stale", got)
	}
	if got := call("PATCH", review, `{"revision":0,"resolution":"Confirmed"}`, a.AdminToken, true); got != 204 {
		t.Fatal("resolve", got)
	}
	if got := call("PATCH", review, `{"revision":0,"resolution":"Changed"}`, a.AdminToken, true); got != 409 {
		t.Fatal("duplicate resolution", got)
	}
	state, err = a.tournaments(ctx)
	if err != nil || state[0].Matches[0].Dispute.Resolution != "Confirmed" || state[0].Matches[0].Dispute.Revision != 1 {
		t.Fatalf("review %+v %v", state, err)
	}
}
