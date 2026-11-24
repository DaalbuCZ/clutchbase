package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestManualPlayerStatsAreScopedAndRevisionProtected(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	const other = "76561198154367262"
	const room = "ABCDEFGHIJKLMNOPQRSTUVWX23"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('server','Server','localhost',27015,'',ARRAY[$1]::text[])`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,'One'),($2,'Two')`, steam, other); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status) VALUES($1,'server','Match','de_mirage','closed','complete')`, room); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,'ct'),($1,$3,'t')`, room, steam, other); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	socket := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var live protocol.Workspace
	readLive(t, socket, &live)
	call := func(id, token, body string, want int) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/stats/"+id, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s: got %d want %d: %s", body, res.Code, want, res.Body.String())
		}
	}
	call(steam, "wrong", `{"kills":12,"deaths":5,"assists":3,"revision":0}`, 401)
	call(steam, a.AdminToken, `{"kills":12,"deaths":5,"revision":0}`, 400)
	call(steam, a.AdminToken, `{"kills":1000,"deaths":5,"assists":3,"revision":0}`, 400)
	call(steam, a.AdminToken, `{"kills":-1,"deaths":5,"assists":3,"revision":0}`, 400)
	call(steam, a.AdminToken, `{"kills":12,"deaths":5,"assists":3,"revision":0}`, 204)
	readLive(t, socket, &live)
	p := live.Matches[0].Participants[0]
	if p.Stats == nil || p.Stats.Kills != 12 || p.Stats.Deaths != 5 || p.Stats.Assists != 3 || live.Matches[0].ResultRevision != 1 {
		t.Fatalf("live stats: %+v", live.Matches[0])
	}
	player, err := a.playerMatches(ctx, steam)
	if err != nil || len(player) != 1 || player[0].Participants[0].Stats == nil || player[0].Participants[0].SteamID != "" {
		t.Fatalf("player projection: %+v %v", player, err)
	}
	history, err := a.matchHistory(ctx, steam, "")
	if err != nil || len(history.Matches) != 1 || history.Matches[0].Participants[0].Stats == nil {
		t.Fatalf("history: %+v %v", history, err)
	}
	hidden, err := a.playerMatches(ctx, other)
	if err != nil || len(hidden) != 0 {
		t.Fatalf("private match exposed: %+v %v", hidden, err)
	}
	call(other, a.AdminToken, `{"kills":1,"deaths":2,"assists":3,"revision":0}`, 409)
	call("76561198154367263", a.AdminToken, `{"kills":1,"deaths":2,"assists":3,"revision":1}`, 404)
	call(steam, a.AdminToken, `{"clear":true,"revision":1}`, 204)
	matches, err := a.matches(ctx)
	if err != nil || matches[0].Participants[0].Stats != nil || matches[0].ResultRevision != 2 {
		t.Fatalf("cleared: %+v %v", matches, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='open' WHERE id=$1`, room); err != nil {
		t.Fatal(err)
	}
	call(steam, a.AdminToken, `{"kills":1,"deaths":2,"assists":3,"revision":2}`, 409)
}
