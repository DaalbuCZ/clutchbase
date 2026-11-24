package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlayerSummariesRespectAccessAndScoreSources(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const first = "76561198154367261"
	const second = "76561198154367262"
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Alpha'),($2,'Bravo')`, first, second)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY[$1,$2]::text[])`, first, second)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,reported_at,reported_ct,reported_t,result_ct,result_t) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Single','de_mirage','closed','complete',now(),13,8,9,13)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side,reported_kills,reported_deaths,reported_assists,kills,deaths,assists) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23',$1,'ct',10,8,3,11,8,3),('ABCDEFGHIJKLMNOPQRSTUVWX23',$2,'t',NULL,NULL,NULL,NULL,NULL,NULL)`, first, second)
	// Final-map CT/T scores do not identify the original rosters; use the clinched series for W/L.
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,series_maps,reported_at,reported_ct,reported_t,reported_team1_score,reported_team2_score) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23','host','Series','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],now(),8,13,2,1),('CDEFGHIJKLMNOPQRSTUVWXYZ23','host','Unresolved series','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],now(),13,8,1,1)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('BCDEFGHIJKLMNOPQRSTUVWXY23',$1,'ct'),('CDEFGHIJKLMNOPQRSTUVWXYZ23',$1,'ct')`, first)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status) VALUES('DEFGHIJKLMNOPQRSTUVWXYZA23','host','Open','de_mirage','open','complete'),('EFGHIJKLMNOPQRSTUVWXYZAB23','host','Unloaded','de_mirage','closed','')`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('DEFGHIJKLMNOPQRSTUVWXYZA23',$1,'ct'),('EFGHIJKLMNOPQRSTUVWXYZAB23',$1,'ct')`, first)
	page, err := a.playerSummaries(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Players) != 1 {
		t.Fatalf("other player's data exposed: %+v", page)
	}
	p := page.Players[0]
	if p.Rooms != 3 || p.ScoredRooms != 2 || p.ManualScores != 1 || p.Wins != 1 || p.Losses != 1 || p.Draws != 0 || p.StatsRooms != 1 || p.ManualStats != 1 || p.Kills == nil || *p.Kills != 11 {
		t.Fatalf("bad source aggregation: %+v", p)
	}
	if len(p.Maps) != 1 || p.Maps[0].Map != "de_mirage" || p.Maps[0].Rooms != 1 || p.Maps[0].Losses != 1 || p.MapExcludedRooms != 2 {
		t.Fatalf("series incorrectly included in map performance: %+v", p)
	}
	other, err := a.playerSummaries(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Players) != 1 || other.Players[0].Kills != nil || other.Players[0].Deaths != nil || other.Players[0].StatsRooms != 0 {
		t.Fatalf("missing stats became zero: %+v", other)
	}
	exec(`UPDATE hosts SET whitelist=ARRAY[$1]::text[] WHERE id='host'`, second)
	page, err = a.playerSummaries(ctx, first)
	if err != nil || len(page.Players) != 0 {
		t.Fatalf("revoked access leaked stats: %+v %v", page, err)
	}
	admin, err := a.playerSummaries(ctx, "")
	if err != nil || len(admin.Players) != 2 {
		t.Fatalf("admin summary: %+v %v", admin, err)
	}
	exec(`UPDATE hosts SET removed_at=now() WHERE id='host'`)
	other, err = a.playerSummaries(ctx, second)
	if err != nil || len(other.Players) != 0 {
		t.Fatalf("removed host leaked stats: %+v %v", other, err)
	}
	admin, err = a.playerSummaries(ctx, "")
	if err != nil || len(admin.Players) != 2 {
		t.Fatalf("admin lost retained stats: %+v %v", admin, err)
	}
}

func TestPlayerStatisticsLiveAccessRevocation(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	token := strings.Repeat("p", 52)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Friend')`, steam)
	exec(`INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('host','Host','localhost',27015,'',ARRAY[$1]::text[])`, steam)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,result_ct,result_t) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage','closed','complete',13,9)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23',$1,'ct')`, steam)
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	denied := liveDial(t, server.URL+"/api/live?view=player-stats", "", nil)
	var unauthorized struct {
		Code int `json:"code"`
	}
	readLive(t, denied, &unauthorized)
	if unauthorized.Code != 401 {
		t.Fatal("statistics stream accepted missing player session")
	}
	c := liveDial(t, server.URL+"/api/live?view=player-stats", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	var page playerSummaryPage
	readLive(t, c, &page)
	if len(page.Players) != 1 || page.Players[0].Wins != 1 {
		t.Fatalf("missing private summary: %+v", page)
	}
	exec(`UPDATE hosts SET whitelist=ARRAY[]::text[] WHERE id='host'`)
	readLive(t, c, &page)
	if len(page.Players) != 0 {
		t.Fatalf("statistics remained visible after access revocation: %+v", page)
	}
	c.CloseNow()
}
