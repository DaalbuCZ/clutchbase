package control

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMatchHistoryPaginationPrivacyAndLiveUpdates(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,whitelist,removed_at) VALUES
      ('visible','Visible','localhost',27015,'',ARRAY[$1]::text[],NULL),
      ('hidden','Hidden','localhost',27025,'',ARRAY[]::text[],NULL),
      ('removed','Removed','localhost',27035,'',ARRAY[$1]::text[],now())`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,'Player')`, steam); err != nil {
		t.Fatal(err)
	}
	token := rand.Text() + rand.Text()
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status,created_at,result_ct,result_t)
      SELECT 'visible-match-'||lpad(i::text,6,'0'),'visible','Match','de_mirage','closed','complete',now(),13,9 FROM generate_series(1,30) i;
      INSERT INTO match_rooms(id,host_id,title,map,status,load_status) VALUES
      ('hidden-match-000001','hidden','Private','de_mirage','closed','complete'),
      ('removed-match-00001','removed','Archived','de_mirage','closed','complete'),
      ('failed-match-000001','visible','Failed','de_mirage','closed','failed')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES('visible-match-000030',$1,'ct')`, steam); err != nil {
		t.Fatal(err)
	}
	first, err := a.matchHistory(ctx, steam, "")
	if err != nil || len(first.Matches) != 25 || first.NextCursor == "" {
		t.Fatalf("first: %+v %v", first, err)
	}
	if first.Matches[0].CanEditResult {
		t.Fatal("player was offered result editing")
	}
	playerSearch, err := a.matchHistory(ctx, steam, "", "player")
	if err != nil || len(playerSearch.Matches) != 1 || playerSearch.Matches[0].ID != "visible-match-000030" {
		t.Fatalf("player name search: %+v %v", playerSearch, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET series_maps=ARRAY['de_mirage','de_nuke','de_inferno'] WHERE id IN ('visible-match-000030','hidden-match-000001')`); err != nil {
		t.Fatal(err)
	}
	seriesSearch, err := a.matchHistory(ctx, steam, "", "NUKE")
	if err != nil || len(seriesSearch.Matches) != 1 || seriesSearch.Matches[0].ID != "visible-match-000030" {
		t.Fatalf("series map search: %+v %v", seriesSearch, err)
	}
	privateSearch, err := a.matchHistory(ctx, steam, "", "private")
	if err != nil || len(privateSearch.Matches) != 0 {
		t.Fatalf("private search: %+v %v", privateSearch, err)
	}
	next, err := a.matchHistory(ctx, steam, first.NextCursor)
	if err != nil || len(next.Matches) != 5 || next.NextCursor != "" {
		t.Fatalf("next: %+v %v", next, err)
	}
	seen := map[string]bool{}
	for _, page := range []historyPage{first, next} {
		for _, m := range page.Matches {
			if seen[m.ID] || m.HostName != "Visible" {
				t.Fatalf("duplicate or private match: %+v", m)
			}
			seen[m.ID] = true
		}
	}
	inaccessible, err := a.matchHistory(ctx, steam, "hidden-match-000001")
	if err != nil || len(inaccessible.Matches) != 0 {
		t.Fatalf("private cursor: %+v %v", inaccessible, err)
	}
	admin, err := a.matchHistory(ctx, "", "")
	if err != nil || len(admin.Matches) != 25 {
		t.Fatalf("admin: %+v %v", admin, err)
	}
	for _, m := range admin.Matches {
		if m.CanEditResult == m.RemovedHost {
			t.Fatalf("wrong history edit permission: %+v", m)
		}
	}
	adminNext, err := a.matchHistory(ctx, "", admin.NextCursor)
	if err != nil || len(adminNext.Matches) != 7 {
		t.Fatalf("admin next: %+v %v", adminNext, err)
	}
	foundRemoved := false
	for _, page := range []historyPage{admin, adminNext} {
		for _, m := range page.Matches {
			if m.RemovedHost {
				foundRemoved = true
			}
		}
	}
	if !foundRemoved {
		t.Fatal("removed server's match missing for admin")
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	conn := liveDial(t, srv.URL+"/api/live?view=player-history", "", http.Header{"Cookie": {playerCookie + "=" + token}})
	var live historyPage
	readLive(t, conn, &live)
	if len(live.Matches) != 25 || len(live.Matches[0].Participants) != 1 || live.Matches[0].Participants[0].SteamID != "" {
		t.Fatalf("player history projection: %+v", live)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET result_ct=14 WHERE id='visible-match-000030'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, conn, &live)
	if live.Matches[0].ResultCT == nil || *live.Matches[0].ResultCT != 14 {
		t.Fatalf("live update: %+v", live.Matches[0])
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY[]::text[] WHERE id='visible'`); err != nil {
		t.Fatal(err)
	}
	readLive(t, conn, &live)
	if len(live.Matches) != 0 {
		t.Fatal("history retained after access revoked")
	}
}
