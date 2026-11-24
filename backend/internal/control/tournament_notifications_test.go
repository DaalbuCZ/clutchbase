package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestTeamAndTournamentChangesReachLiveClients(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	token := strings.Repeat("p", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES('76561198154367261','Friend')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,'76561198154367261',now()+interval '1 hour')`, digest(token)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	admin := liveDial(t, server.URL+"/api/live?view=manage", a.AdminToken, nil)
	defer admin.CloseNow()
	player := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	defer player.CloseNow()
	var manage protocol.Workspace
	var view playerWorkspace
	readLive(t, admin, &manage)
	readLive(t, player, &view)
	if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES('team','Friends','76561198154367261'); INSERT INTO team_members(team_id,steam_id) VALUES('team','76561198154367261')`); err != nil {
		t.Fatal(err)
	}
	readLive(t, admin, &manage)
	readLive(t, player, &view)
	if len(manage.Teams) != 1 || manage.Teams[0].Name != "Friends" || view.Team == nil || view.Team.Name != "Friends" {
		t.Fatalf("team update missing: admin=%+v player=%+v", manage.Teams, view.Team)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Friday Cup',1,'open')`); err != nil {
		t.Fatal(err)
	}
	readLive(t, admin, &manage)
	readLive(t, player, &view)
	if len(manage.Tournaments) != 1 || manage.Tournaments[0].Name != "Friday Cup" || len(view.Tournaments) != 1 || view.Tournaments[0].Name != "Friday Cup" {
		t.Fatalf("tournament update missing: admin=%+v player=%+v", manage.Tournaments, view.Tournaments)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE tournaments SET status='checkin' WHERE id='cup'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, admin, &manage)
	readLive(t, player, &view)
	if manage.Tournaments[0].Status != "checkin" || view.Tournaments[0].Status != "checkin" {
		t.Fatal("tournament status did not update")
	}
}
