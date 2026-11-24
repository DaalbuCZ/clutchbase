package control

import (
	"context"
	"encoding/json"
	"time"

	"clutchbase/internal/protocol"
)

type historyMatch struct {
	SeriesMaps     []historySeriesMap          `json:"seriesMaps"`
	SeriesTeam1    *int                        `json:"seriesTeam1"`
	SeriesTeam2    *int                        `json:"seriesTeam2"`
	ID             string                      `json:"id"`
	Title          string                      `json:"title"`
	HostName       string                      `json:"hostName"`
	Map            string                      `json:"map"`
	CreatedAt      time.Time                   `json:"createdAt"`
	ResultCT       *int                        `json:"resultCT"`
	ResultT        *int                        `json:"resultT"`
	ReportedCT     *int                        `json:"reportedCT"`
	ReportedT      *int                        `json:"reportedT"`
	ResultRevision int64                       `json:"resultRevision"`
	CanEditResult  bool                        `json:"canEditResult"`
	RemovedHost    bool                        `json:"removedHost"`
	SeriesMatch    bool                        `json:"seriesMatch"`
	Participants   []protocol.MatchParticipant `json:"participants"`
}

type historySeriesMap struct {
	Number int    `json:"number"`
	Map    string `json:"map"`
	Team1  *int   `json:"team1"`
	Team2  *int   `json:"team2"`
}

type historyPage struct {
	CapturedAt string         `json:"capturedAt"`
	Matches    []historyMatch `json:"matches"`
	NextCursor string         `json:"nextCursor"`
}

func (a *API) matchHistory(ctx context.Context, steamID, cursor string, terms ...string) (historyPage, error) {
	search := ""
	if len(terms) > 0 {
		search = terms[0]
	}
	page := historyPage{CapturedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Matches: []historyMatch{}}
	rows, err := a.DB.Query(ctx, `SELECT m.id,m.title,h.name,m.map,m.created_at,m.result_ct,m.result_t,m.reported_ct,m.reported_t,m.result_revision,($1='' AND h.removed_at IS NULL),h.removed_at IS NOT NULL,`+matchParticipantsSQL(steamID == "")+`,cardinality(m.series_maps)>1,m.reported_team1_score,m.reported_team2_score,
		COALESCE((SELECT json_agg(json_build_object('number',s.ord,'map',s.map,'team1',r.team1_score,'team2',r.team2_score) ORDER BY s.ord) FROM unnest(m.series_maps) WITH ORDINALITY s(map,ord) LEFT JOIN match_series_map_results r ON r.match_id=m.id AND r.map_number=s.ord WHERE cardinality(m.series_maps)>1),'[]'::json)
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		WHERE m.status='closed' AND m.load_status='complete'
		AND ($1='' OR (h.removed_at IS NULL AND $1=ANY(h.whitelist)))
		AND ($3='' OR strpos(lower(m.title),lower($3))>0 OR strpos(lower(h.name),lower($3))>0 OR strpos(lower(m.map),lower($3))>0
		 OR EXISTS (SELECT 1 FROM unnest(m.series_maps) AS s(map) WHERE strpos(lower(s.map),lower($3))>0)
		 OR EXISTS (SELECT 1 FROM match_room_players p JOIN players pl ON pl.steam_id=p.steam_id WHERE p.match_id=m.id AND strpos(lower(pl.game_name),lower($3))>0))
		AND ($2='' OR (m.created_at,m.id) < (SELECT old.created_at,old.id FROM match_rooms old JOIN hosts oh ON oh.id=old.host_id
			WHERE old.id=$2 AND old.status='closed' AND old.load_status='complete' AND ($1='' OR (oh.removed_at IS NULL AND $1=ANY(oh.whitelist)))))
		ORDER BY m.created_at DESC,m.id DESC LIMIT 26`, steamID, cursor, search)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var m historyMatch
		var participants, maps []byte
		if err := rows.Scan(&m.ID, &m.Title, &m.HostName, &m.Map, &m.CreatedAt, &m.ResultCT, &m.ResultT, &m.ReportedCT, &m.ReportedT, &m.ResultRevision, &m.CanEditResult, &m.RemovedHost, &participants, &m.SeriesMatch, &m.SeriesTeam1, &m.SeriesTeam2, &maps); err != nil {
			return page, err
		}
		if err := json.Unmarshal(participants, &m.Participants); err != nil {
			return page, err
		}
		if err := json.Unmarshal(maps, &m.SeriesMaps); err != nil {
			return page, err
		}
		page.Matches = append(page.Matches, m)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Matches) > 25 {
		page.Matches = page.Matches[:25]
		page.NextCursor = page.Matches[24].ID
	}
	return page, nil
}
