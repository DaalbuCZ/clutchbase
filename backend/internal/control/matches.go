package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

// Once a load is requested, show the names captured in that request. Profiles
// can still change independently while MatchZy is processing it.
func matchParticipantsSQL(includeSteamID bool) string {
	steamID := ""
	if includeSteamID {
		steamID = "'steamId',pl.steam_id,"
	}
	return `COALESCE((SELECT json_agg(json_build_object(` + steamID + `
    'gameName', CASE WHEN m.load_status IN ('pending','complete') THEN COALESCE(
        (SELECT item.value->>'gameName' FROM jsonb_array_elements(
            CASE WHEN jsonb_typeof(m.load_payload->rp.side)='array' THEN m.load_payload->rp.side ELSE '[]'::jsonb END
        ) AS item(value) WHERE item.value->>'steamId'=rp.steam_id LIMIT 1),
        COALESCE(NULLIF(pl.game_name,''),'Player '||right(pl.steam_id,4)))
      ELSE COALESCE(NULLIF(pl.game_name,''),'Player '||right(pl.steam_id,4)) END,
    'side',rp.side,'ready',rp.ready,'stats',CASE WHEN rp.kills IS NULL THEN NULL ELSE json_build_object('kills',rp.kills,'deaths',rp.deaths,'assists',rp.assists) END,
    'reportedStats',CASE WHEN rp.reported_kills IS NULL THEN NULL ELSE json_build_object('kills',rp.reported_kills,'deaths',rp.reported_deaths,'assists',rp.reported_assists) END) ORDER BY rp.joined_at,rp.steam_id)
    FROM match_room_players rp JOIN players pl ON pl.steam_id=rp.steam_id
    WHERE rp.match_id=m.id),'[]'::json)`
}

// Keep actionable rooms regardless of age, plus the latest 50 ordinary history entries.
func selectedMatchesSQL(player bool) string {
	filter := ""
	if player {
		filter = " AND $1=ANY(h.whitelist)"
	}
	return `WITH relevant AS (
		SELECT m.id,m.created_at,
			(m.status='open' OR m.load_status IN ('pending','failed') OR
			(m.load_status='complete' AND m.ended_at IS NULL AND m.load_command_id<>'' AND m.load_command_id=h.active_match_setup_id)) AS priority
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE h.removed_at IS NULL` + filter + `
	), selected AS (
		SELECT id,priority,sum((NOT priority)::int) OVER (ORDER BY created_at DESC,id DESC) AS history_rank FROM relevant
	) `
}

const setupUnloadedSQL = `m.status='open' AND m.load_status='complete' AND m.ended_at IS NULL
	AND h.desired_state='running' AND h.observed_state='running' AND h.last_seen>now()-interval '20 seconds'
	AND h.driver='managed' AND h.active_preset='match' AND h.access_status='verified' AND h.error=''
	AND NOT h.remove_requested AND h.match_phase='idle' AND ` + roomMapAvailableSQL + ` AND h.can_load_match AND (NOT m.manual_ready OR h.supports_manual_ready) AND h.active_match_setup_id=''
	AND h.match_control_status<>'pending' AND h.mode_command_status<>'pending'`

// Scores are current-map CT/T observations, never a final result or starting-roster score.
const liveScoreSQL = `CASE WHEN m.status='open' AND m.load_status='complete' AND m.ended_at IS NULL
 AND m.load_command_id<>'' AND m.load_command_id=h.active_match_setup_id
 AND h.live_score->>'setupId'=m.load_command_id AND h.live_score->>'map'=h.active_map
 AND h.last_seen>now()-interval '20 seconds' AND h.desired_state='running' AND h.observed_state='running'
 AND h.driver='managed' AND h.active_preset='match' AND h.access_status='verified' AND h.error='' AND NOT h.remove_requested
 AND h.match_phase IN ('live','paused')
 AND (h.live_score->>'observedAt')::bigint>extract(epoch FROM now()-interval '15 seconds')*1000
 AND (h.live_score->>'observedAt')::bigint<=extract(epoch FROM now()+interval '1 second')*1000
 THEN h.live_score ELSE NULL END`

func (a *API) matches(ctx context.Context) ([]protocol.Match, error) {
	rows, err := a.DB.Query(ctx, selectedMatchesSQL(false)+`SELECT m.pause_vote_threshold,(SELECT count(*)::int FROM match_pause_votes pv WHERE pv.match_id=m.id),h.revision,m.id,m.host_id,h.name,m.title,m.map,m.overtime,m.manual_ready,m.status,m.players_per_side,
		count(p.steam_id)::int,count(p.steam_id) FILTER(WHERE p.side='ct')::int,
		count(p.steam_id) FILTER(WHERE p.side='t')::int,count(p.steam_id) FILTER(WHERE p.ready)::int,
		`+matchParticipantsSQL(true)+`,`+matchVotesSQL+`,
		m.created_at,m.load_status,m.load_error,m.ended_at,m.settings_revision,m.result_ct,m.result_t,m.result_revision,m.reported_ct,m.reported_t,(`+setupUnloadedSQL+`),m.cancel_status,m.cancel_error,(`+canCancelMatchSQL+`),EXISTS (SELECT 1 FROM match_rooms previous WHERE previous.host_id=h.id AND previous.load_status='failed'),
		(m.load_status='complete' AND m.ended_at IS NULL AND COALESCE((SELECT supports_match_control FROM machines WHERE id=h.machine_id),false) AND m.load_command_id<>'' AND m.load_command_id=h.active_match_setup_id AND h.desired_state='running' AND h.observed_state='running' AND h.last_seen>now()-interval '20 seconds' AND h.driver='managed' AND h.active_preset='match' AND h.access_status='verified' AND h.error='' AND NOT h.remove_requested AND h.match_phase IN ('setup','live','paused') AND h.match_control_status<>'pending' AND h.mode_command_status<>'pending'),(`+matchLoadEligibleSQL+` AND count(p.steam_id)>0 AND count(p.steam_id) FILTER(WHERE p.side='ct')>0 AND count(p.steam_id) FILTER(WHERE p.side='t')>0 AND count(p.steam_id) FILTER(WHERE p.side='ct')<=m.players_per_side AND count(p.steam_id) FILTER(WHERE p.side='t')<=m.players_per_side AND count(p.ready) FILTER(WHERE p.ready)=count(p.steam_id)),NOT EXISTS(SELECT 1 FROM tournament_matches tm WHERE tm.room_id=m.id),cardinality(m.series_maps)>1,`+liveScoreSQL+`,`+matchLoadBlockersSQL+` FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		LEFT JOIN match_room_players p ON p.match_id=m.id
		WHERE h.removed_at IS NULL AND m.id IN (SELECT id FROM selected WHERE priority OR history_rank<=50) GROUP BY m.id,h.id,h.name,h.desired_state,h.observed_state,h.last_seen,h.driver,h.active_preset,h.access_status,h.can_load_match ORDER BY m.created_at DESC,m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []protocol.Match{}
	for rows.Next() {
		var m protocol.Match
		var participants, votes []byte
		var canRemovePlayer bool
		if err := rows.Scan(&m.PauseVoteThreshold, &m.PauseVotes, &m.ControlRevision, &m.ID, &m.HostID, &m.HostName, &m.Title, &m.Map, &m.Overtime, &m.ManualReady, &m.Status, &m.PlayersPerSide, &m.Players, &m.CTPlayers, &m.TPlayers, &m.Ready, &participants, &votes, &m.CreatedAt, &m.LoadStatus, &m.LoadError, &m.EndedAt, &m.SettingsRevision, &m.ResultCT, &m.ResultT, &m.ResultRevision, &m.ReportedCT, &m.ReportedT, &m.SetupUnloaded, &m.CancelStatus, &m.CancelError, &m.CanCancelLoad, &m.LoadBlockedByFailure, &m.CanEnd, &m.CanLoad, &canRemovePlayer, &m.SeriesMatch, &m.LiveScore, &m.LoadBlockers); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(participants, &m.Participants); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(votes, &m.MapVotes); err != nil {
			return nil, err
		}
		m.CanConfigurePauseVoting = m.Status == "open"
		m.CanClose = m.Status == "open" && m.LoadStatus != "pending"
		m.CanReopen = m.Status == "closed" && m.LoadStatus == "" && canRemovePlayer
		m.CanEdit = m.Status == "open" && m.LoadStatus == "" && canRemovePlayer
		m.CanRemovePlayer = m.CanEdit && canRemovePlayer
		m.CanShuffle = m.CanEdit && m.Players >= 2 && m.Players <= 2*m.PlayersPerSide
		m.CanEditResult = m.Status == "closed" && m.LoadStatus == "complete"
		out = append(out, m)
	}
	return out, rows.Err()
}

func canOpenMatch(h protocol.Host) bool {
	return h.RestartPhase != "stopping" && h.RestartPhase != "starting" && h.RestartPhase != "cancelling" && h.Online && h.Status == "running" && h.Driver == "managed" && h.ActivePreset == "match" && h.AccessStatus == "verified" && h.Whitelist != nil && protocol.ValidInstalledMap(h.ActiveMap) && h.CanObserveMap
}

func (a *API) createMatch(w http.ResponseWriter, r *http.Request) {
	a.createMatchFor(w, r, "")
}

func (a *API) playerCreateMatch(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	a.createMatchFor(w, r, player.SteamID)
}

func (a *API) createMatchFor(w http.ResponseWriter, r *http.Request, steam string) {
	var in struct {
		HostID         string `json:"hostId"`
		Title          string `json:"title"`
		PlayersPerSide int    `json:"playersPerSide"`
		Map            string `json:"map"`
		Overtime       bool   `json:"overtime"`
		ManualReady    bool   `json:"manualReady"`
		Side           string `json:"side"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.PlayersPerSide == 0 {
		in.PlayersPerSide = 5
	}
	if in.HostID == "" || len(in.Title) < 1 || len(in.Title) > 80 || in.PlayersPerSide < 1 || in.PlayersPerSide > 5 {
		fail(w, 400, "Choose a server, title, and room size from 1v1 to 5v5")
		return
	}
	if in.Map != "" && !protocol.ValidInstalledMap(in.Map) {
		fail(w, 400, "Choose a supported map")
		return
	}
	if steam != "" && in.Side != "ct" && in.Side != "t" {
		fail(w, 400, "Choose a side")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	h, err := lockHost(ctx, tx, in.HostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if steam != "" {
		allowed := false
		for _, id := range h.Whitelist {
			if id == steam {
				allowed = true
				break
			}
		}
		if !allowed || !h.AllowPlayerRooms {
			fail(w, 404, "Server unavailable for player-created rooms")
			return
		}
	}
	if !h.CanObserveMap {
		fail(w, 409, "An updated machine agent that reports the current map is required to open a match room")
		return
	}
	if !canOpenMatch(h) || (steam != "" && playerJoinBlocker(h) != "") {
		fail(w, 409, "A running server with verified Match mode and a saved SteamID roster is required")
		return
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND (status='open' OR load_status='pending' OR cancel_status='pending'))`, h.ID).Scan(&busy); err != nil {
		internal(w, err)
		return
	}
	if busy {
		fail(w, 409, "This server already has an open or unresolved match room")
		return
	}
	matchMap := h.ActiveMap
	if in.Map != "" {
		matchMap = in.Map
	}
	if !hostHasMap(h, matchMap) {
		fail(w, 409, "Map is not currently available on this server")
		return
	}
	id := rand.Text()
	var m protocol.Match
	m.Participants = []protocol.MatchParticipant{}
	m.CanClose = true
	m.CanEdit = true
	m.CanRemovePlayer = true
	m.SettingsRevision = 1
	err = tx.QueryRow(ctx, `INSERT INTO match_rooms(id,host_id,title,map,players_per_side,overtime,manual_ready,created_by_steam_id) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($9,''))
		RETURNING id,host_id,$8,title,map,overtime,manual_ready,status,players_per_side,0,0,0,0,created_at`, id, h.ID, in.Title, matchMap, in.PlayersPerSide, in.Overtime, in.ManualReady, h.Name, steam).Scan(&m.ID, &m.HostID, &m.HostName, &m.Title, &m.Map, &m.Overtime, &m.ManualReady, &m.Status, &m.PlayersPerSide, &m.Players, &m.CTPlayers, &m.TPlayers, &m.Ready, &m.CreatedAt)
	if err != nil {
		fail(w, 409, "This server already has an open match room")
		return
	}
	if steam != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,$3)`, id, steam, in.Side); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND load_status='failed')", h.ID).Scan(&m.LoadBlockedByFailure); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	if steam != "" {
		respond(w, 201, map[string]string{"id": id})
		return
	}
	respond(w, 201, m)
}

func (a *API) updateMatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status        string `json:"status"`
		ConfirmLoaded bool   `json:"confirmLoaded"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "closed" {
		fail(w, 400, "Matches can only be closed")
		return
	}
	tag, err := a.DB.Exec(r.Context(), "UPDATE match_rooms SET status='closed',closed_at=now() WHERE id=$1 AND status='open' AND load_status<>'pending' AND (load_status<>'complete' OR $2)", r.PathValue("id"), in.ConfirmLoaded)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Room is closed, still loading, or requires confirmation that closing it does not end the game")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) loadMatch(w http.ResponseWriter, r *http.Request) {
	a.loadMatchFor(w, r, "")
}
func (a *API) playerLoadMatch(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	a.loadMatchFor(w, r, player.SteamID)
}
func (a *API) loadMatchFor(w http.ResponseWriter, r *http.Request, steam string) {
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var locked string
	// Lock the host first, consistently with match-control requests.
	if err = tx.QueryRow(ctx, "SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 FOR UPDATE OF h", r.PathValue("id")).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Open match room not found")
			return
		}
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, "SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Open match room not found")
			return
		}
		internal(w, err)
		return
	}
	if steam != "" {
		allowed, err := a.playerRoomPermission(ctx, tx, r.PathValue("id"), steam, true)
		if err != nil {
			internal(w, err)
			return
		}
		if !allowed {
			fail(w, 404, "Room unavailable for player management")
			return
		}
	}
	var id, hostID, mapName string
	var seriesMaps, seriesSides []string
	var size, players, ct, t, ready int
	var eligible, overtime, manualReady bool
	err = tx.QueryRow(ctx, `SELECT m.id,m.host_id,m.map,m.overtime,m.players_per_side,count(p.steam_id)::int,count(p.steam_id) FILTER(WHERE p.side='ct')::int,count(p.steam_id) FILTER(WHERE p.side='t')::int,count(p.steam_id) FILTER(WHERE p.ready)::int,
		(`+matchLoadEligibleSQL+`)
		,m.series_maps,m.series_sides,m.manual_ready
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id LEFT JOIN match_room_players p ON p.match_id=m.id WHERE m.id=$1 GROUP BY m.id,h.id`, r.PathValue("id")).Scan(&id, &hostID, &mapName, &overtime, &size, &players, &ct, &t, &ready, &eligible, &seriesMaps, &seriesSides, &manualReady)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Open match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	for _, selected := range append([]string{mapName}, seriesMaps...) {
		if protocol.ValidWorkshopMap(selected) {
			fail(w, 409, "Workshop match loading is not supported by the current reporter")
			return
		}
	}
	if !eligible || players == 0 || ct == 0 || t == 0 || ct > size || t > size || ready != players {
		fail(w, 409, "Both sides must have ready players and the server must be available in Match mode")
		return
	}
	h, err := lockHost(ctx, tx, hostID)
	if err != nil {
		internal(w, err)
		return
	}
	for _, selected := range append([]string{mapName}, seriesMaps...) {
		if !hostHasMap(h, selected) {
			fail(w, 409, "Map is not currently available on this server")
			return
		}
	}
	var fenceRequired bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT h.supports_match_fence AND m.supports_match_cancel AND m.supports_match_setup_expiry FROM hosts h JOIN machines m ON m.id=h.machine_id WHERE h.id=$1),false)`, hostID).Scan(&fenceRequired); err != nil {
		internal(w, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT p.steam_id,COALESCE(NULLIF(p.game_name,''),'Player '||right(p.steam_id,4)),rp.side FROM match_room_players rp JOIN players p ON p.steam_id=rp.steam_id WHERE rp.match_id=$1 ORDER BY rp.joined_at`, id)
	if err != nil {
		internal(w, err)
		return
	}
	if len(seriesMaps) == 0 {
		seriesMaps = []string{mapName}
		seriesSides = []string{"team1_ct"}
	}
	if len(seriesMaps) != len(seriesSides) || (len(seriesMaps) != 1 && len(seriesMaps) != 3 && len(seriesMaps) != 5) || seriesMaps[0] != mapName {
		fail(w, 409, "The match map sequence is invalid")
		return
	}
	setup := protocol.MatchSetup{ID: rand.Text(), RequestedAt: time.Now().UnixMilli(), FenceRequired: fenceRequired, ManualReady: manualReady, Map: mapName, Maps: seriesMaps, MapSides: seriesSides, Overtime: overtime, PlayersPerSide: size, CT: []protocol.MatchSetupPlayer{}, T: []protocol.MatchSetupPlayer{}}
	for rows.Next() {
		var p protocol.MatchSetupPlayer
		var side string
		if err = rows.Scan(&p.SteamID, &p.GameName, &side); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		if side == "ct" {
			setup.CT = append(setup.CT, p)
		} else if side == "t" {
			setup.T = append(setup.T, p)
		} else {
			rows.Close()
			fail(w, 409, "Every player must choose a side")
			return
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, "SELECT count(*)::int FROM match_room_players WHERE match_id=$1 AND ready=false", id).Scan(&ready); err != nil {
		internal(w, err)
		return
	}
	if ready != 0 {
		fail(w, 409, "Every player must be ready before loading the match")
		return
	}
	payload, err := json.Marshal(setup)
	if err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET load_command_id=$2,load_status='pending',load_error='',load_requested_at=now(),load_payload=$3::jsonb,load_retry_after=NULL,load_fence_required=$5,cancel_id='',cancel_status='',cancel_error='',load_expiry_enforced=COALESCE((SELECT machines.supports_match_setup_expiry FROM machines JOIN hosts ON hosts.machine_id=machines.id WHERE hosts.id=$4),false) WHERE id=$1`, id, setup.ID, string(payload), hostID, fenceRequired); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusAccepted, map[string]string{"status": "pending"})
}

type playerMatch struct {
	PauseVoteThreshold int                         `json:"pauseVoteThreshold"`
	PauseVotes         int                         `json:"pauseVotes"`
	MyPauseVote        bool                        `json:"myPauseVote"`
	RosterFingerprint  string                      `json:"rosterFingerprint,omitempty"`
	SettingsRevision   int64                       `json:"settingsRevision,omitempty"`
	MyRoom             bool                        `json:"myRoom"`
	MapVotes           []protocol.MapVote          `json:"mapVotes"`
	MyMapVote          string                      `json:"myMapVote"`
	LiveScore          *protocol.LiveScore         `json:"liveScore"`
	ControlRevision    int64                       `json:"controlRevision"`
	MatchPhase         string                      `json:"matchPhase"`
	ControlStatus      string                      `json:"controlStatus"`
	ControlAction      string                      `json:"controlAction"`
	ControlError       string                      `json:"controlError"`
	SeriesMatch        bool                        `json:"seriesMatch"`
	ReportedCT         *int                        `json:"reportedCT"`
	ReportedT          *int                        `json:"reportedT"`
	SetupUnloaded      bool                        `json:"setupUnloaded"`
	ResultCT           *int                        `json:"resultCT"`
	ResultT            *int                        `json:"resultT"`
	ID                 string                      `json:"id"`
	HostID             string                      `json:"hostId"`
	HostName           string                      `json:"hostName"`
	Title              string                      `json:"title"`
	Map                string                      `json:"map"`
	Overtime           bool                        `json:"overtime"`
	ManualReady        bool                        `json:"manualReady"`
	Status             string                      `json:"status"`
	LoadStatus         string                      `json:"loadStatus"`
	LoadError          string                      `json:"loadError"`
	LoadBlockers       []string                    `json:"loadBlockers"`
	EndedAt            *time.Time                  `json:"endedAt"`
	PlayersPerSide     int                         `json:"playersPerSide"`
	Players            int                         `json:"players"`
	Ready              int                         `json:"ready"`
	CTPlayers          int                         `json:"ctPlayers"`
	TPlayers           int                         `json:"tPlayers"`
	Joined             bool                        `json:"joined"`
	MyReady            bool                        `json:"myReady"`
	MySide             string                      `json:"mySide"`
	CanJoin            bool                        `json:"canJoin"`
	Actions            playerMatchActions          `json:"actions"`
	Participants       []protocol.MatchParticipant `json:"participants"`
}

type playerMatchActions struct {
	Reopen     bool `json:"reopen"`
	Shuffle    bool `json:"shuffle"`
	Edit       bool `json:"edit"`
	Load       bool `json:"load"`
	Close      bool `json:"close"`
	VoteMap    bool `json:"voteMap"`
	Report     bool `json:"report"`
	Pause      bool `json:"pause"`
	Resume     bool `json:"resume"`
	JoinCT     bool `json:"joinCT"`
	JoinT      bool `json:"joinT"`
	SwitchSide bool `json:"switchSide"`
	Ready      bool `json:"ready"`
	Unready    bool `json:"unready"`
	Leave      bool `json:"leave"`
}

func (a *API) playerMatches(ctx context.Context, steam string) ([]playerMatch, error) {
	rows, err := a.DB.Query(ctx, selectedMatchesSQL(true)+`SELECT m.pause_vote_threshold,(SELECT count(*)::int FROM match_pause_votes pv WHERE pv.match_id=m.id),EXISTS(SELECT 1 FROM match_pause_votes pv WHERE pv.match_id=m.id AND pv.steam_id=$1),m.id,h.id,h.name,m.title,m.map,m.overtime,m.manual_ready,m.status,m.load_status,m.load_error,m.players_per_side,count(p.steam_id)::int,
		count(p.steam_id) FILTER(WHERE p.ready)::int,count(p.steam_id) FILTER(WHERE p.side='ct')::int,
		count(p.steam_id) FILTER(WHERE p.side='t')::int,
		COALESCE(bool_or(p.steam_id=$1),false),COALESCE(bool_or(p.steam_id=$1 AND p.ready),false),
		COALESCE(max(p.side) FILTER(WHERE p.steam_id=$1),''),COALESCE(m.created_by_steam_id=$1,false),m.ended_at,m.result_ct,m.result_t,m.reported_ct,m.reported_t,(`+setupUnloadedSQL+`),
		`+matchParticipantsSQL(false)+`,`+matchVotesSQL+`,COALESCE((SELECT map FROM match_map_votes WHERE match_id=m.id AND steam_id=$1),''),
		(m.status='open' AND m.load_status='' AND h.observed_state='running' AND h.desired_state='running'
		AND h.last_seen>now()-interval '20 seconds' AND h.driver='managed'
		AND h.active_preset='match' AND h.access_status='verified'
		AND (count(p.steam_id) FILTER(WHERE p.side='ct')<m.players_per_side
		OR count(p.steam_id) FILTER(WHERE p.side='t')<m.players_per_side
		OR COALESCE(bool_or(p.steam_id=$1),false))),cardinality(m.series_maps)>1,
		EXISTS(SELECT 1 FROM tournament_matches tm WHERE tm.room_id=m.id),
		NOT EXISTS(SELECT 1 FROM match_reports report WHERE report.match_id=m.id AND report.reporter_steam_id=$1),
        h.revision,CASE WHEN h.last_seen>now()-interval '20 seconds' AND h.observed_state='running' AND h.desired_state='running' AND h.access_status='verified' AND h.active_preset='match' THEN h.match_phase ELSE 'unknown' END,h.match_control_status,COALESCE(h.match_control_action,''),h.match_control_error,
        (m.status='open' AND m.load_status='complete' AND m.ended_at IS NULL AND m.load_command_id<>'' AND m.load_command_id=h.active_match_setup_id),
		COALESCE((h.last_seen>now()-interval '20 seconds' AND h.machine_id IS NOT NULL AND h.driver='managed' AND h.error='' AND h.desired_state='running' AND h.observed_state='running' AND h.active_preset='match' AND h.access_status='verified' AND NOT h.remove_requested AND h.mode_command_status<>'pending' AND h.match_control_status<>'pending'),false),h.can_pause_match,h.can_resume_match,`+liveScoreSQL+`,`+matchLoadBlockersSQL+`
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		LEFT JOIN match_room_players p ON p.match_id=m.id
		WHERE h.removed_at IS NULL AND $1=ANY(h.whitelist) AND m.id IN (SELECT id FROM selected WHERE priority OR history_rank<=50)
		GROUP BY m.id,h.id ORDER BY m.created_at DESC,m.id DESC`, steam)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []playerMatch{}
	for rows.Next() {
		var m playerMatch
		var participants, votes []byte
		var tournamentRoom, notReported, activeControlRoom, canControl, canPause, canResume bool
		if err := rows.Scan(&m.PauseVoteThreshold, &m.PauseVotes, &m.MyPauseVote, &m.ID, &m.HostID, &m.HostName, &m.Title, &m.Map, &m.Overtime, &m.ManualReady, &m.Status, &m.LoadStatus, &m.LoadError, &m.PlayersPerSide, &m.Players, &m.Ready, &m.CTPlayers, &m.TPlayers, &m.Joined, &m.MyReady, &m.MySide, &m.MyRoom, &m.EndedAt, &m.ResultCT, &m.ResultT, &m.ReportedCT, &m.ReportedT, &m.SetupUnloaded, &participants, &votes, &m.MyMapVote, &m.CanJoin, &m.SeriesMatch, &tournamentRoom, &notReported, &m.ControlRevision, &m.MatchPhase, &m.ControlStatus, &m.ControlAction, &m.ControlError, &activeControlRoom, &canControl, &canPause, &canResume, &m.LiveScore, &m.LoadBlockers); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(participants, &m.Participants); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(votes, &m.MapVotes); err != nil {
			return nil, err
		}
		editable := m.Status == "open" && m.LoadStatus == ""
		m.Actions.VoteMap = m.Joined && editable && !tournamentRoom
		m.Actions.Report = m.Joined && notReported
		m.CanJoin = m.CanJoin && !tournamentRoom
		m.Actions.Pause = m.Joined && !m.MyPauseVote && activeControlRoom && canControl && canPause && m.MatchPhase == "live"
		m.Actions.Resume = m.Joined && activeControlRoom && canControl && canResume && m.MatchPhase == "paused"
		if !activeControlRoom {
			m.MatchPhase = "unknown"
			m.ControlStatus = ""
			m.ControlAction = ""
			m.ControlError = ""
		}
		m.Actions.JoinCT = !m.Joined && m.CanJoin && m.CTPlayers < m.PlayersPerSide
		m.Actions.JoinT = !m.Joined && m.CanJoin && m.TPlayers < m.PlayersPerSide
		m.Actions.SwitchSide = m.Joined && editable && !tournamentRoom && !m.MyReady && ((m.MySide == "ct" && m.TPlayers < m.PlayersPerSide) || (m.MySide != "ct" && m.CTPlayers < m.PlayersPerSide))
		m.Actions.Ready = m.Joined && editable && !m.MyReady && (m.MySide == "ct" || m.MySide == "t")
		m.Actions.Unready = m.Joined && editable && m.MyReady
		m.Actions.Leave = m.Joined && editable && !tournamentRoom
		out = append(out, m)
	}
	return out, rows.Err()
}

func (a *API) joinMatch(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Side string `json:"side"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Side != "ct" && in.Side != "t" {
		fail(w, 400, "Choose CT or T side")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var canJoin bool
	var playersPerSide int
	err = tx.QueryRow(ctx, `SELECT m.status='open' AND m.load_status='' AND h.observed_state='running' AND h.desired_state='running'
		AND h.last_seen>now()-interval '20 seconds' AND h.driver='managed' AND h.active_preset='match'
		AND h.access_status='verified' AND COALESCE($2=ANY(h.whitelist),false)
		AND NOT EXISTS(SELECT 1 FROM tournament_matches tm WHERE tm.room_id=m.id),m.players_per_side
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.id=$1 FOR UPDATE OF m`, r.PathValue("id"), p.SteamID).Scan(&canJoin, &playersPerSide)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !canJoin {
		fail(w, 409, "Match is closed or its server is not ready for players")
		return
	}
	var hasRoom bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_room_players WHERE match_id=$1 AND steam_id=$2)
		OR (SELECT count(*) FROM match_room_players WHERE match_id=$1 AND side=$3)<$4`, r.PathValue("id"), p.SteamID, in.Side, playersPerSide).Scan(&hasRoom); err != nil {
		internal(w, err)
		return
	}
	if !hasRoom {
		fail(w, 409, "That side is full")
		return
	}
	tag, err := tx.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,$3) ON CONFLICT(match_id,steam_id) DO NOTHING`, r.PathValue("id"), p.SteamID, in.Side)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() > 0 {
		if _, err = tx.Exec(ctx, "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND steam_id<>$2 AND ready=true", r.PathValue("id"), p.SteamID); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setMatchSide(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Side string `json:"side"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Side != "ct" && in.Side != "t" {
		fail(w, 400, "Choose CT or T side")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var open bool
	var playersPerSide int
	err = tx.QueryRow(ctx, "SELECT status='open' AND load_status='',players_per_side FROM match_rooms WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&open, &playersPerSide)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !open {
		fail(w, 409, "Match is closed")
		return
	}
	var tournamentRoom bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=$1)", r.PathValue("id")).Scan(&tournamentRoom); err != nil {
		internal(w, err)
		return
	}
	if tournamentRoom {
		fail(w, 409, "Tournament fixture sides are assigned from the checked-in team rosters")
		return
	}
	var previousSide string
	err = tx.QueryRow(ctx, "SELECT side FROM match_room_players WHERE match_id=$1 AND steam_id=$2", r.PathValue("id"), p.SteamID).Scan(&previousSide)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Join the match before choosing a side")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE match_room_players SET side=$3
		WHERE match_id=$1 AND steam_id=$2 AND ready=false AND EXISTS(
		SELECT 1 FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		WHERE m.id=$1 AND h.removed_at IS NULL AND $2=ANY(h.whitelist))
		AND (side=$3 OR (SELECT count(*) FROM match_room_players p WHERE p.match_id=$1 AND p.side=$3)<$4)`, r.PathValue("id"), p.SteamID, in.Side, playersPerSide)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Join the match, unready, and choose a side with space")
		return
	}
	if previousSide != in.Side {
		if _, err = tx.Exec(ctx, "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND steam_id<>$2 AND ready=true", r.PathValue("id"), p.SteamID); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) leaveMatch(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var editable bool
	err = tx.QueryRow(r.Context(), "SELECT status='open' AND load_status='' AND NOT EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=match_rooms.id) FROM match_rooms WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&editable)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !editable {
		fail(w, 409, "Tournament rosters are fixed; other room rosters lock after closure or a load request")
		return
	}
	tag, err := tx.Exec(r.Context(), "DELETE FROM match_room_players WHERE match_id=$1 AND steam_id=$2", r.PathValue("id"), p.SteamID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() > 0 {
		if _, err = tx.Exec(r.Context(), "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready=true", r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setMatchReady(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Ready *bool `json:"ready"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Ready == nil {
		fail(w, 400, "Ready state required")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var open bool
	err = tx.QueryRow(ctx, "SELECT status='open' AND load_status='' FROM match_rooms WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !open {
		fail(w, 409, "Match is closed")
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE match_room_players SET ready=$3
		WHERE match_id=$1 AND steam_id=$2 AND ($3=false OR (side IN ('ct','t') AND EXISTS(
		SELECT 1 FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		WHERE m.id=$1 AND h.removed_at IS NULL AND $2=ANY(h.whitelist))))`, r.PathValue("id"), p.SteamID, *in.Ready)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Join the match before marking ready")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) requirePlayerMutation(w http.ResponseWriter, r *http.Request) (playerIdentity, bool) {
	if a.origin() == "" || r.Header.Get("Origin") != a.origin() {
		fail(w, 403, "Same-origin request required")
		return playerIdentity{}, false
	}
	p, err := a.session(r.Context(), r)
	if errors.Is(err, errNoSession) {
		fail(w, 401, "Sign in with Steam")
		return playerIdentity{}, false
	}
	if err != nil {
		internal(w, err)
		return playerIdentity{}, false
	}
	return p, true
}
