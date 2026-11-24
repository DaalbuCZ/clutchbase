package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLeaderboardAccessRankingAndLiveRevocation(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const viewer = "76561198154367261"
	const alpha = "76561198154367262"
	const bravo = "76561198154367263"
	const hidden = "76561198154367264"
	token := strings.Repeat("p", 52)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Viewer'),($2,'Alpha'),($3,'Bravo'),($4,'Hidden')`, viewer, alpha, bravo, hidden)
	exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), viewer)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('visible','Visible','localhost',27015,'',ARRAY[$1]::text[]),('private','Private','localhost',27025,'',ARRAY[$2]::text[])`, viewer, hidden)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,result_ct,result_t,reported_ct,reported_t,reported_at) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','visible','One','de_mirage','closed','complete',13,7,1,13,now()),('BCDEFGHIJKLMNOPQRSTUVWXY23','visible','Two','de_mirage','closed','complete',13,7,NULL,NULL,NULL),('CDEFGHIJKLMNOPQRSTUVWXYZ23','visible','Unscored','de_mirage','closed','complete',NULL,NULL,NULL,NULL,NULL),('DEFGHIJKLMNOPQRSTUVWXYZA23','private','Private','de_mirage','closed','complete',13,7,NULL,NULL,NULL)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23',$1,'ct'),('ABCDEFGHIJKLMNOPQRSTUVWX23',$2,'t'),('BCDEFGHIJKLMNOPQRSTUVWXY23',$2,'ct'),('CDEFGHIJKLMNOPQRSTUVWXYZ23',$1,'ct'),('DEFGHIJKLMNOPQRSTUVWXYZA23',$3,'ct')`, alpha, bravo, hidden)
	page, err := a.leaderboard(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Players) != 2 || page.Players[0].Name != "Alpha" || page.Players[0].Rank != 1 || page.Players[0].Wins != 1 || page.Players[0].Rooms != 2 || page.Players[1].Name != "Bravo" || page.Players[1].Rank != 1 || page.Players[1].Losses != 1 {
		t.Fatalf("unexpected public ranking: %+v", page)
	}
	admin, err := a.leaderboard(ctx, "")
	if err != nil || len(admin.Players) != 3 {
		t.Fatalf("admin ranking: %+v %v", admin, err)
	}
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,series_maps,reported_team1_score,reported_team2_score) VALUES('EFGHIJKLMNOPQRSTUVWXYZAB23','visible','Series','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],2,1),('FGHIJKLMNOPQRSTUVWXYZABC23','visible','Unresolved','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],1,1)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('EFGHIJKLMNOPQRSTUVWXYZAB23',$1,'ct'),('FGHIJKLMNOPQRSTUVWXYZABC23',$1,'ct')`, alpha)
	page, err = a.leaderboard(ctx, viewer)
	if err != nil || page.Players[0].Wins != 2 || page.Players[0].Rooms != 4 || page.Players[1].Rank != 2 {
		t.Fatalf("series results or rankings incorrect: %+v %v", page, err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	deniedAdmin := liveDial(t, server.URL+"/api/live?view=leaderboard", "", nil)
	var adminAuth struct {
		Code int `json:"code"`
	}
	readLive(t, deniedAdmin, &adminAuth)
	if adminAuth.Code != 401 {
		t.Fatal("missing administrator token accepted")
	}
	deniedAdmin.CloseNow()
	denied := liveDial(t, server.URL+"/api/live?view=player-leaderboard", "", nil)
	var unauth struct {
		Code int `json:"code"`
	}
	readLive(t, denied, &unauth)
	if unauth.Code != 401 {
		t.Fatal("missing session accepted")
	}
	denied.CloseNow()
	c := liveDial(t, server.URL+"/api/live?view=player-leaderboard", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	defer c.CloseNow()
	readLive(t, c, &page)
	if len(page.Players) != 2 {
		t.Fatalf("missing ranking: %+v", page)
	}
	exec(`UPDATE hosts SET whitelist=ARRAY[]::text[] WHERE id='visible'`)
	readLive(t, c, &page)
	if len(page.Players) != 0 {
		t.Fatalf("ranking remained after access revoked: %+v", page)
	}
}
