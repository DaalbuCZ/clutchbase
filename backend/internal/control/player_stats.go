package control

import (
	"context"

	"github.com/jackc/pgx/v5"
	"time"
)

type playerSummary struct {
	SteamID          string       `json:"steamId"`
	GameName         string       `json:"gameName"`
	Rooms            int          `json:"rooms"`
	ScoredRooms      int          `json:"scoredRooms"`
	ManualScores     int          `json:"manualScores"`
	Wins             int          `json:"wins"`
	Losses           int          `json:"losses"`
	Draws            int          `json:"draws"`
	StatsRooms       int          `json:"statsRooms"`
	ManualStats      int          `json:"manualStats"`
	Kills            *int64       `json:"kills"`
	Deaths           *int64       `json:"deaths"`
	Assists          *int64       `json:"assists"`
	Maps             []mapSummary `json:"maps"`
	MapExcludedRooms int          `json:"mapExcludedRooms"`
}

type mapSummary struct {
	Map         string `json:"map"`
	Rooms       int    `json:"rooms"`
	ScoredRooms int    `json:"scoredRooms"`
	Wins        int    `json:"wins"`
	Losses      int    `json:"losses"`
	Draws       int    `json:"draws"`
}

type playerSummaryPage struct {
	CapturedAt string          `json:"capturedAt"`
	Players    []playerSummary `json:"players"`
	Truncated  bool            `json:"truncated"`
}

func (a *API) playerSummaries(ctx context.Context, steam string) (playerSummaryPage, error) {
	page := playerSummaryPage{CapturedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Players: []playerSummary{}}
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return page, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `WITH visible AS (
		SELECT rp.steam_id,rp.side,rp.kills,rp.deaths,rp.assists,rp.reported_kills,rp.reported_deaths,rp.reported_assists,
			CASE WHEN cardinality(m.series_maps)<=1 THEN m.result_ct END AS result_ct,
            CASE WHEN cardinality(m.series_maps)>1 THEN
                CASE WHEN ((m.reported_team1_score=cardinality(m.series_maps)/2+1 AND m.reported_team2_score<cardinality(m.series_maps)/2+1) OR (m.reported_team2_score=cardinality(m.series_maps)/2+1 AND m.reported_team1_score<cardinality(m.series_maps)/2+1)) THEN m.reported_team1_score END
                ELSE COALESCE(m.result_ct,m.reported_ct) END AS ct,
            CASE WHEN cardinality(m.series_maps)>1 THEN
                CASE WHEN ((m.reported_team1_score=cardinality(m.series_maps)/2+1 AND m.reported_team2_score<cardinality(m.series_maps)/2+1) OR (m.reported_team2_score=cardinality(m.series_maps)/2+1 AND m.reported_team1_score<cardinality(m.series_maps)/2+1)) THEN m.reported_team2_score END
                ELSE COALESCE(m.result_t,m.reported_t) END AS t
		FROM match_room_players rp JOIN match_rooms m ON m.id=rp.match_id JOIN hosts h ON h.id=m.host_id
		WHERE m.status='closed' AND m.load_status='complete' AND
			($1='' OR (rp.steam_id=$1 AND h.removed_at IS NULL AND $1=ANY(h.whitelist)))
	)
	SELECT p.steam_id,COALESCE(NULLIF(p.game_name,''),'Player '||right(p.steam_id,4)),count(*)::int,
		count(*) FILTER (WHERE v.ct IS NOT NULL)::int,count(*) FILTER (WHERE v.result_ct IS NOT NULL)::int,
		count(*) FILTER (WHERE (v.side='ct' AND v.ct>v.t) OR (v.side='t' AND v.t>v.ct))::int,
		count(*) FILTER (WHERE (v.side='ct' AND v.ct<v.t) OR (v.side='t' AND v.t<v.ct))::int,
		count(*) FILTER (WHERE v.ct=v.t)::int,
		count(*) FILTER (WHERE COALESCE(v.kills,v.reported_kills) IS NOT NULL)::int,
		count(*) FILTER (WHERE v.kills IS NOT NULL)::int,
		sum(COALESCE(v.kills,v.reported_kills)),sum(COALESCE(v.deaths,v.reported_deaths)),sum(COALESCE(v.assists,v.reported_assists))
	FROM visible v JOIN players p ON p.steam_id=v.steam_id GROUP BY p.steam_id,p.game_name
	ORDER BY count(*) DESC,lower(p.game_name),p.steam_id LIMIT 201`, steam)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var p playerSummary
		if err := rows.Scan(&p.SteamID, &p.GameName, &p.Rooms, &p.ScoredRooms, &p.ManualScores, &p.Wins, &p.Losses, &p.Draws, &p.StatsRooms, &p.ManualStats, &p.Kills, &p.Deaths, &p.Assists); err != nil {
			return page, err
		}
		page.Players = append(page.Players, p)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Players) > 200 {
		page.Players = page.Players[:200]
		page.Truncated = true
	}
	rows.Close()
	ids := make([]string, 0, len(page.Players))
	index := make(map[string]int, len(page.Players))
	for i := range page.Players {
		ids = append(ids, page.Players[i].SteamID)
		index[page.Players[i].SteamID] = i
		page.Players[i].Maps = []mapSummary{}
		page.Players[i].MapExcludedRooms = page.Players[i].Rooms
	}
	mapRows, err := tx.Query(ctx, `WITH visible AS (
		SELECT rp.steam_id,m.id,m.map,m.series_maps,rp.side,COALESCE(m.result_ct,m.reported_ct) ct,COALESCE(m.result_t,m.reported_t) t
		FROM match_room_players rp JOIN match_rooms m ON m.id=rp.match_id JOIN hosts h ON h.id=m.host_id
		WHERE m.status='closed' AND m.load_status='complete'
		AND rp.steam_id=ANY($1::text[]) AND ($2='' OR (rp.steam_id=$2 AND h.removed_at IS NULL AND $2=ANY(h.whitelist)))
	), appearances AS (
		SELECT steam_id,id,map,side,ct,t FROM visible WHERE cardinality(series_maps)<=1 AND map<>''
		UNION ALL
		SELECT v.steam_id,v.id,v.series_maps[r.map_number],v.side,r.team1_score,r.team2_score
		FROM visible v JOIN match_series_map_results r ON r.match_id=v.id
		WHERE cardinality(v.series_maps)>1 AND r.map_number<=cardinality(v.series_maps) AND v.series_maps[r.map_number]<>''
	), included AS (
		SELECT steam_id,count(DISTINCT id)::int AS rooms FROM appearances GROUP BY steam_id
	) SELECT a.steam_id,a.map,count(*)::int,count(*) FILTER (WHERE a.ct IS NOT NULL AND a.t IS NOT NULL)::int,
		count(*) FILTER (WHERE (a.side='ct' AND a.ct>a.t) OR (a.side='t' AND a.t>a.ct))::int,
		count(*) FILTER (WHERE (a.side='ct' AND a.ct<a.t) OR (a.side='t' AND a.t<a.ct))::int,
		count(*) FILTER (WHERE a.ct=a.t)::int,i.rooms FROM appearances a JOIN included i ON i.steam_id=a.steam_id GROUP BY a.steam_id,a.map,i.rooms ORDER BY a.steam_id,a.map`, ids, steam)
	if err != nil {
		return page, err
	}
	defer mapRows.Close()
	for mapRows.Next() {
		var id string
		var m mapSummary
		var includedRooms int
		if err := mapRows.Scan(&id, &m.Map, &m.Rooms, &m.ScoredRooms, &m.Wins, &m.Losses, &m.Draws, &includedRooms); err != nil {
			return page, err
		}
		p := &page.Players[index[id]]
		p.Maps = append(p.Maps, m)
		p.MapExcludedRooms = p.Rooms - includedRooms
	}
	if err := mapRows.Err(); err != nil {
		return page, err
	}

	mapRows.Close()
	if err := tx.Commit(ctx); err != nil {
		return page, err
	}
	return page, nil
}
