package control

import (
	"context"
	"testing"
)

func TestPlayerSummariesIncludeRecordedSeriesMaps(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const (
		viewer = "76561198154367261"
		other  = "76561198154367262"
	)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	exec(`INSERT INTO players(steam_id,game_name) VALUES($1,'Viewer'),($2,'Other')`, viewer, other)
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,whitelist,removed_at) VALUES
		('series-visible','Visible','localhost',27015,'',ARRAY[$1,$2]::text[],NULL),
		('series-hidden','Hidden','localhost',27025,'',ARRAY[$2]::text[],NULL),
		('series-removed','Removed','localhost',27035,'',ARRAY[$1]::text[],now())`, viewer, other)

	// The empty map row represents a legacy room with no selected map. The
	// current write path prevents this, but retained history must still classify it.
	exec(`ALTER TABLE match_rooms DROP CONSTRAINT IF EXISTS match_rooms_map_check`)
	exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,series_maps,series_sides,result_ct,result_t) VALUES
		('series-recorded-stats-room','series-visible','Recorded','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno','de_ancient','de_anubis'],ARRAY['team1_ct','team1_t','team1_ct','team1_t','team1_ct'],NULL,NULL),
		('series-unrecorded-stats-room','series-visible','Unrecorded','de_mirage','closed','complete',ARRAY['de_mirage','de_nuke','de_inferno'],ARRAY['team1_ct','team1_t','team1_ct'],NULL,NULL),
		('single-map-stats-room','series-visible','Single','de_dust2','closed','complete',ARRAY[]::text[],ARRAY[]::text[],13,8),
		('no-map-stats-room','series-visible','No map','', 'closed','complete',ARRAY[]::text[],ARRAY[]::text[],NULL,NULL),
		('hidden-series-stats-room','series-hidden','Hidden','de_mirage','closed','complete',ARRAY['de_inferno','de_nuke','de_ancient'],ARRAY['team1_ct','team1_t','team1_ct'],NULL,NULL),
		('removed-series-stats-room','series-removed','Removed','de_mirage','closed','complete',ARRAY['de_train','de_vertigo','de_anubis'],ARRAY['team1_ct','team1_t','team1_ct'],NULL,NULL)`)
	exec(`INSERT INTO match_room_players(match_id,steam_id,side) VALUES
		('series-recorded-stats-room',$1,'ct'),('series-recorded-stats-room',$2,'t'),
		('series-unrecorded-stats-room',$1,'ct'),('series-unrecorded-stats-room',$2,'t'),
		('single-map-stats-room',$1,'ct'),('single-map-stats-room',$2,'t'),
		('no-map-stats-room',$1,'ct'),('no-map-stats-room',$2,'t'),
		('hidden-series-stats-room',$1,'ct'),('hidden-series-stats-room',$2,'t'),
		('removed-series-stats-room',$1,'ct'),('removed-series-stats-room',$2,'t')`, viewer, other)
	// Team 1 is the original CT side. Map 2 switches the live side, but its
	// result remains team 1's result for the original roster.
	exec(`INSERT INTO match_series_map_results(match_id,map_number,team1_score,team2_score) VALUES
		('series-recorded-stats-room',1,13,7),
		('series-recorded-stats-room',2,7,13),
		('series-recorded-stats-room',3,12,12),
		('hidden-series-stats-room',1,13,7),
		('removed-series-stats-room',1,13,7)`)

	page, err := a.playerSummaries(ctx, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Players) != 1 {
		t.Fatalf("private or removed series data exposed: %+v", page)
	}
	p := page.Players[0]
	if p.Rooms != 4 || p.MapExcludedRooms != 2 {
		t.Fatalf("series/no-map coverage: %+v", p)
	}
	if len(p.Maps) != 4 {
		t.Fatalf("configured but unrecorded maps were included: %+v", p.Maps)
	}
	want := map[string]mapSummary{
		"de_dust2":   {Map: "de_dust2", Rooms: 1, ScoredRooms: 1, Wins: 1},
		"de_inferno": {Map: "de_inferno", Rooms: 1, ScoredRooms: 1, Draws: 1},
		"de_mirage":  {Map: "de_mirage", Rooms: 1, ScoredRooms: 1, Wins: 1},
		"de_nuke":    {Map: "de_nuke", Rooms: 1, ScoredRooms: 1, Losses: 1},
	}
	for _, got := range p.Maps {
		if got != want[got.Map] {
			t.Fatalf("map %s: got %+v want %+v", got.Map, got, want[got.Map])
		}
		delete(want, got.Map)
	}
	if len(want) != 0 {
		t.Fatalf("missing map summaries: %+v", want)
	}

	admin, err := a.playerSummaries(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	adminViewer := findPlayerSummary(t, admin, viewer)
	if adminViewer.Rooms != 6 || adminViewer.MapExcludedRooms != 2 {
		t.Fatalf("admin did not retain hidden/removed history: %+v", adminViewer)
	}
	if got := findMapSummary(t, adminViewer, "de_train"); got.Rooms != 1 || got.Wins != 1 || got.ScoredRooms != 1 {
		t.Fatalf("admin hidden/removed map history: %+v", got)
	}
	if got := findMapSummary(t, adminViewer, "de_inferno"); got.Rooms != 2 || got.Wins != 1 || got.Draws != 1 || got.ScoredRooms != 2 {
		t.Fatalf("admin hidden map history: %+v", got)
	}

	otherPage, err := a.playerSummaries(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	otherPlayer := findPlayerSummary(t, otherPage, other)
	if otherPlayer.Rooms != 5 || otherPlayer.MapExcludedRooms != 2 {
		t.Fatalf("opponent visibility or coverage: %+v", otherPlayer)
	}
	if got := findMapSummary(t, otherPlayer, "de_inferno"); got.Rooms != 2 || got.Losses != 1 || got.Draws != 1 || got.ScoredRooms != 2 {
		t.Fatalf("opponent original-side result mapping: %+v", got)
	}
	if got := findMapSummary(t, otherPlayer, "de_mirage"); got.Rooms != 1 || got.Losses != 1 || got.ScoredRooms != 1 {
		t.Fatalf("opponent T-side loss: %+v", got)
	}
	if got := findMapSummary(t, otherPlayer, "de_nuke"); got.Rooms != 1 || got.Wins != 1 || got.ScoredRooms != 1 {
		t.Fatalf("opponent T-side win: %+v", got)
	}
}

func findPlayerSummary(t *testing.T, page playerSummaryPage, steam string) playerSummary {
	t.Helper()
	for _, player := range page.Players {
		if player.SteamID == steam {
			return player
		}
	}
	t.Fatalf("player %q missing from %+v", steam, page)
	return playerSummary{}
}

func findMapSummary(t *testing.T, player playerSummary, mapName string) mapSummary {
	t.Helper()
	for _, m := range player.Maps {
		if m.Map == mapName {
			return m
		}
	}
	t.Fatalf("map %q missing from %+v", mapName, player.Maps)
	return mapSummary{}
}
