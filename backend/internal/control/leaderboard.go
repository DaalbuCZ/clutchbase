package control

import (
	"context"
	"time"
)

type leaderboardEntry struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Wins   int    `json:"wins"`
	Losses int    `json:"losses"`
	Draws  int    `json:"draws"`
	Rooms  int    `json:"rooms"`
}

type leaderboardPage struct {
	CapturedAt string             `json:"capturedAt"`
	Players    []leaderboardEntry `json:"players"`
	Truncated  bool               `json:"truncated"`
}

func (a *API) leaderboard(ctx context.Context, steam string) (leaderboardPage, error) {
	page := leaderboardPage{CapturedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Players: []leaderboardEntry{}}
	rows, err := a.DB.Query(ctx, `WITH visible AS (
		SELECT rp.steam_id,rp.side,
		CASE WHEN cardinality(m.series_maps)>1 THEN CASE WHEN ((m.reported_team1_score=cardinality(m.series_maps)/2+1 AND m.reported_team2_score<cardinality(m.series_maps)/2+1) OR (m.reported_team2_score=cardinality(m.series_maps)/2+1 AND m.reported_team1_score<cardinality(m.series_maps)/2+1)) THEN m.reported_team1_score END ELSE COALESCE(m.result_ct,m.reported_ct) END AS ct,
		CASE WHEN cardinality(m.series_maps)>1 THEN CASE WHEN ((m.reported_team1_score=cardinality(m.series_maps)/2+1 AND m.reported_team2_score<cardinality(m.series_maps)/2+1) OR (m.reported_team2_score=cardinality(m.series_maps)/2+1 AND m.reported_team1_score<cardinality(m.series_maps)/2+1)) THEN m.reported_team2_score END ELSE COALESCE(m.result_t,m.reported_t) END AS t
		FROM match_room_players rp JOIN match_rooms m ON m.id=rp.match_id JOIN hosts h ON h.id=m.host_id
		WHERE m.status='closed' AND m.load_status='complete' AND ($1='' OR (h.removed_at IS NULL AND $1=ANY(h.whitelist)))
	), totals AS (
		SELECT p.steam_id,COALESCE(NULLIF(p.game_name,''),'Player '||right(p.steam_id,4)) name,count(*)::int rooms,
		count(*) FILTER (WHERE (v.side='ct' AND v.ct>v.t) OR (v.side='t' AND v.t>v.ct))::int wins,
		count(*) FILTER (WHERE (v.side='ct' AND v.ct<v.t) OR (v.side='t' AND v.t<v.ct))::int losses,
		count(*) FILTER (WHERE v.ct=v.t)::int draws
		FROM visible v JOIN players p ON p.steam_id=v.steam_id GROUP BY p.steam_id,p.game_name
		HAVING count(*) FILTER (WHERE v.ct IS NOT NULL AND v.t IS NOT NULL)>0
	) SELECT rank() OVER (ORDER BY wins DESC)::int,name,wins,losses,draws,rooms FROM totals ORDER BY wins DESC,lower(name),steam_id LIMIT 201`, steam)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry leaderboardEntry
		if err := rows.Scan(&entry.Rank, &entry.Name, &entry.Wins, &entry.Losses, &entry.Draws, &entry.Rooms); err != nil {
			return page, err
		}
		page.Players = append(page.Players, entry)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Players) > 200 {
		page.Players = page.Players[:200]
		page.Truncated = true
	}
	return page, nil
}
