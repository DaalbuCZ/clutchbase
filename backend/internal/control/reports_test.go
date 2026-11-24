package control

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMatchReportsAuthorizationAndLiveReview(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const first = "76561198154367261"
	const second = "76561198154367262"
	const outsider = "76561198154367263"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	tokens := map[string]string{first: strings.Repeat("a", 52), second: strings.Repeat("b", 52), outsider: strings.Repeat("c", 52)}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES('76561198154367261','Alpha'),('76561198154367262','Bravo'),('76561198154367263','Other'); INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY['76561198154367261','76561198154367262','76561198154367263']::text[]); INSERT INTO match_rooms(id,host_id,title,map) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Friday','de_mirage'); INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367261','ct'),('ABCDEFGHIJKLMNOPQRSTUVWX23','76561198154367262','t')`); err != nil {
		t.Fatal(err)
	}
	for steam, token := range tokens {
		if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
			t.Fatal(err)
		}
	}
	call := func(steam, origin, body string) int {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/player/matches/"+room+"/report", strings.NewReader(body))
		if steam != "" {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: tokens[steam]})
		}
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := call("", a.Origin, `{"description":"Problem"}`); got != 401 {
		t.Fatalf("missing session %d", got)
	}
	if got := call(first, "https://attacker.invalid", `{"description":"Problem"}`); got != 403 {
		t.Fatalf("wrong origin %d", got)
	}
	if got := call(first, a.Origin, `{"description":"  "}`); got != 400 {
		t.Fatalf("empty description %d", got)
	}
	if got := call(outsider, a.Origin, `{"description":"Problem"}`); got != 403 {
		t.Fatalf("nonparticipant %d", got)
	}
	before, err := a.playerMatches(ctx, first)
	if err != nil || len(before) != 1 || !before[0].Actions.Report {
		t.Fatalf("missing report action: %+v %v", before, err)
	}
	if got := call(first, a.Origin, `{"description":"First problem"}`); got != 201 {
		t.Fatalf("submit %d", got)
	}
	after, err := a.playerMatches(ctx, first)
	if err != nil || len(after) != 1 || after[0].Actions.Report {
		t.Fatalf("report action remained: %+v %v", after, err)
	}
	if got := call(first, a.Origin, `{"description":"Again"}`); got != 409 {
		t.Fatalf("duplicate %d", got)
	}
	if got := call(second, a.Origin, `{"description":"Private problem"}`); got != 201 {
		t.Fatalf("second submit %d", got)
	}
	own, err := a.matchReports(ctx, first)
	if err != nil || len(own.Reports) != 1 || own.Reports[0].Description != "First problem" || own.Reports[0].ReporterSteamID != "" {
		t.Fatalf("private reports %+v %v", own, err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player-reports", "", http.Header{"Cookie": {playerCookie + "=" + tokens[first]}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var snapshot reportPage
	readLive(t, c, &snapshot)
	if len(snapshot.Reports) != 1 || snapshot.Reports[0].Status != "open" {
		t.Fatalf("initial %+v", snapshot)
	}
	review := func(token, body string) int {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/reports/"+own.Reports[0].ID, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if got := review("", `{"status":"resolved","revision":1}`); got != 401 {
		t.Fatalf("unauthorized review %d", got)
	}
	if got := review(a.AdminToken, `{"status":"resolved","revision":1}`); got != 204 {
		t.Fatalf("review %d", got)
	}
	readLive(t, c, &snapshot)
	if len(snapshot.Reports) != 1 || snapshot.Reports[0].Status != "resolved" || snapshot.Reports[0].Revision != 2 {
		t.Fatalf("live review %+v", snapshot)
	}
	if got := review(a.AdminToken, `{"status":"open","revision":1}`); got != 409 {
		t.Fatalf("stale revision %d", got)
	}
	admin, err := a.matchReports(ctx, "")
	if err != nil || len(admin.Reports) != 2 || admin.Reports[1].ReporterSteamID != first {
		t.Fatalf("admin reports %+v %v", admin, err)
	}
	if _, err = a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[$1]::text[] WHERE id='host'`, second); err != nil {
		t.Fatal(err)
	}
	if got := call(first, a.Origin, fmt.Sprintf(`{"description":%q}`, "No access")); got != 404 {
		t.Fatalf("revoked access %d", got)
	}
}
