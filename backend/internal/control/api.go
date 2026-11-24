package control

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type API struct {
	DB              *pgxpool.Pool
	AdminToken      string
	Origin          string
	Context         context.Context
	SteamClient     *http.Client
	joinPasswordsMu sync.RWMutex
	joinPasswords   map[string]cachedJoinPassword
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	liveSlots := make(chan struct{}, 64)
	mux.HandleFunc("GET /api/live", func(w http.ResponseWriter, r *http.Request) {
		select {
		case liveSlots <- struct{}{}:
			defer func() { <-liveSlots }()
		default:
			fail(w, 503, "Live connection limit reached")
			return
		}
		a.live(w, r)
	})
	mux.HandleFunc("GET /api/auth/steam", a.steamStart)
	mux.HandleFunc("GET /api/auth/steam/callback", a.steamCallback)
	mux.HandleFunc("GET /api/player/session", a.playerSession)
	mux.HandleFunc("GET /api/player/state", a.playerState)
	mux.HandleFunc("GET /api/player/access-requests", a.playerAccessRequests)
	mux.HandleFunc("POST /api/player/access-requests", a.createPlayerAccessRequest)
	mux.HandleFunc("PATCH /api/player/profile", a.updatePlayerProfile)
	mux.HandleFunc("POST /api/player/logout", a.playerLogout)
	mux.HandleFunc("POST /api/player/matches", a.playerCreateMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/load", a.playerLoadMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/close", a.playerCloseMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/shuffle", a.playerShuffleMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/remove-player", a.playerRemoveMatchPlayer)
	mux.HandleFunc("PATCH /api/player/matches/{id}/settings", a.playerEditMatchSettings)
	mux.HandleFunc("POST /api/player/matches/{id}/join", a.joinMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/leave", a.leaveMatch)
	mux.HandleFunc("POST /api/player/matches/{id}/reopen", a.playerReopenMatch)
	mux.HandleFunc("POST /api/matches/{id}/reopen", a.admin(a.reopenMatch))
	mux.HandleFunc("PATCH /api/player/matches/{id}/side", a.setMatchSide)
	mux.HandleFunc("PATCH /api/player/matches/{id}/ready", a.setMatchReady)
	mux.HandleFunc("POST /api/player/matches/{id}/control", a.playerMatchControl)
	mux.HandleFunc("POST /api/player/matches/{id}/report", a.submitMatchReport)
	mux.HandleFunc("PUT /api/player/matches/{id}/map-vote", a.voteMatchMap)
	mux.HandleFunc("DELETE /api/player/matches/{id}/map-vote", a.voteMatchMap)
	a.registerTournamentRoutes(mux)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if err := a.DB.Ping(r.Context()); err != nil {
			fail(w, 503, "Database unavailable")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/state", a.admin(a.list))
	mux.HandleFunc("GET /api/access-requests", a.admin(a.listAccessRequests))
	mux.HandleFunc("PATCH /api/access-requests/{id}", a.admin(a.updateAccessRequest))
	mux.HandleFunc("PATCH /api/reports/{id}", a.admin(a.reviewMatchReport))
	mux.HandleFunc("POST /api/hosts", a.admin(a.create))
	mux.HandleFunc("PATCH /api/hosts/{id}", a.admin(a.update))
	mux.HandleFunc("GET /api/hosts/{id}/join-password", a.admin(a.getJoinPassword))
	mux.HandleFunc("POST /api/hosts/{id}/mode", a.admin(a.changeLiveMode))
	mux.HandleFunc("POST /api/hosts/{id}/reporter-update", a.admin(a.updateReporter))
	mux.HandleFunc("POST /api/hosts/{id}/game-update", a.admin(a.updateGame))
	mux.HandleFunc("POST /api/hosts/{id}/game-update/cancel", a.admin(a.cancelGameUpdate))
	mux.HandleFunc("POST /api/hosts/{id}/restart", a.admin(a.requestServerRestart))
	mux.HandleFunc("POST /api/hosts/{id}/match-control", a.admin(a.changeMatchControl))
	mux.HandleFunc("PATCH /api/matches/{id}/pause-voting", a.admin(a.setPauseVoting))
	mux.HandleFunc("POST /api/matches/{id}/end", a.admin(a.endMatch))
	mux.HandleFunc("POST /api/matches/{id}/cancel-load", a.admin(a.cancelMatchLoad))
	mux.HandleFunc("DELETE /api/hosts/{id}", a.admin(a.removeServer))
	mux.HandleFunc("POST /api/matches", a.admin(a.createMatch))
	mux.HandleFunc("POST /api/matches/{id}/load", a.admin(a.loadMatch))
	mux.HandleFunc("PATCH /api/matches/{id}", a.admin(a.updateMatch))
	mux.HandleFunc("PATCH /api/matches/{id}/settings", a.admin(a.editMatchSettings))
	mux.HandleFunc("POST /api/matches/{id}/shuffle", a.admin(a.shuffleMatch))
	mux.HandleFunc("PATCH /api/matches/{id}/result", a.admin(a.editMatchResult))
	mux.HandleFunc("PATCH /api/matches/{id}/series-map-result", a.admin(a.editSeriesMapResult))
	mux.HandleFunc("PATCH /api/matches/{id}/stats/{steam}", a.admin(a.editMatchStats))
	mux.HandleFunc("DELETE /api/matches/{id}/players/{steam}", a.admin(a.removeMatchPlayer))
	mux.HandleFunc("POST /api/agent/{id}/heartbeat", a.heartbeat)
	mux.HandleFunc("POST /api/machines", a.admin(a.createMachine))
	mux.HandleFunc("POST /api/machines/{id}/rotate-token", a.admin(a.rotateMachineToken))
	mux.HandleFunc("POST /api/machines/{id}/servers", a.admin(a.createServer))
	mux.HandleFunc("POST /api/machine-agent/{id}/heartbeat", a.machineHeartbeat)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API endpoint not found") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/api/live" {
			mux.ServeHTTP(w, r)
			return
		}
		timeout := 5 * time.Second
		if r.URL.Path == "/api/auth/steam/callback" {
			timeout = 12 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(digest(a)), []byte(digest(b))) == 1
}
func bearer(r *http.Request) string {
	value, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return value
}
func (a *API) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.AdminToken == "" || !equal(bearer(r), a.AdminToken) {
			fail(w, 401, "Administrator token required")
			return
		}
		next(w, r)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeLimit(w, r, v, 8192)
}
func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "Invalid JSON request")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Print("Response write failed")
	}
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func internal(w http.ResponseWriter, err error) {
	log.Printf("Database operation failed: %v", err)
	fail(w, 500, "Database operation failed")
}

const columns = "id, name, address, port, map, desired_state, observed_state, active_map, driver, error, last_seen, revision, COALESCE(machine_id,''), players, max_players, log_tail, access_status, remove_requested, COALESCE((SELECT supports_removal FROM machines WHERE machines.id=hosts.machine_id),false), whitelist, whitelist_hash, COALESCE((SELECT supports_whitelist FROM machines WHERE machines.id=hosts.machine_id),false), preset, applied_preset, active_preset, COALESCE((SELECT supports_presets FROM machines WHERE machines.id=hosts.machine_id),false), can_live_mode, mode_command_id, COALESCE(mode_command_mode,''), mode_command_status, mode_command_error, mode_command_requested_at, can_load_match,match_phase,match_paused,can_pause_match,can_resume_match,match_control_id,COALESCE(match_control_action,''),match_control_status,match_control_error,match_control_requested_at,COALESCE((SELECT supports_map_observation FROM machines WHERE machines.id=hosts.machine_id),false),reporter_source_hash,reporter_installed_hash,reporter_command_id,reporter_requested_hash,reporter_command_status,reporter_command_error,reporter_command_requested_at,COALESCE((SELECT supports_reporter_update FROM machines WHERE machines.id=hosts.machine_id),false),COALESCE((SELECT status FROM game_updates WHERE host_id=hosts.id),''),COALESCE((SELECT error FROM game_updates WHERE host_id=hosts.id),''),COALESCE((SELECT issued_at IS NOT NULL FROM game_updates WHERE host_id=hosts.id),false),COALESCE((SELECT supports_game_update FROM machines WHERE machines.id=hosts.machine_id),false),supports_match_series,game_build_installed,game_build_latest,game_build_checked_at,COALESCE((SELECT phase FROM server_restarts WHERE host_id=hosts.id),''),COALESCE((SELECT supports_restart AND driver='managed' AND last_seen>now()-interval '20 seconds' FROM machines WHERE machines.id=hosts.machine_id),false),allow_player_rooms,supports_manual_ready,workshop_maps,workshop_maps_seen"

func scan(row pgx.Row) (protocol.Host, error) {
	var h protocol.Host
	err := row.Scan(&h.ID, &h.Name, &h.Address, &h.Port, &h.Map, &h.DesiredState, &h.ObservedState, &h.ActiveMap, &h.Driver, &h.Error, &h.LastSeen, &h.Revision, &h.MachineID, &h.Players, &h.MaxPlayers, &h.LogTail, &h.AccessStatus, &h.Removing, &h.CanRemove, &h.Whitelist, &h.WhitelistHash, &h.CanWhitelist, &h.Preset, &h.AppliedPreset, &h.ActivePreset, &h.CanPreset, &h.CanLiveMode, &h.ModeCommandID, &h.ModeCommandMode, &h.ModeCommandStatus, &h.ModeCommandError, &h.ModeCommandRequestedAt, &h.CanLoadMatch, &h.MatchPhase, &h.MatchPaused, &h.CanPauseMatch, &h.CanResumeMatch, &h.MatchControlID, &h.MatchControlAction, &h.MatchControlStatus, &h.MatchControlError, &h.MatchControlRequestedAt, &h.CanObserveMap, &h.ReporterSourceHash, &h.ReporterInstalledHash, &h.ReporterCommandID, &h.ReporterRequestedHash, &h.ReporterUpdateStatus, &h.ReporterUpdateError, &h.ReporterRequestedAt, &h.CanReporterUpdate, &h.GameUpdateStatus, &h.GameUpdateError, &h.GameUpdateIssued, &h.CanUpdateGame, &h.SupportsMatchSeries, &h.GameBuildInstalled, &h.GameBuildLatest, &h.GameBuildCheckedAt, &h.RestartPhase, &h.CanRestart, &h.AllowPlayerRooms, &h.SupportsManualReady, &h.WorkshopMaps, &h.WorkshopMapsSeen)
	h.Project(time.Now())
	return h, err
}

// Lock first so subqueries in columns see changes committed while waiting for the host.
func lockHost(ctx context.Context, tx pgx.Tx, id string) (protocol.Host, error) {
	var locked string
	if err := tx.QueryRow(ctx, "SELECT id FROM hosts WHERE id=$1 AND removed_at IS NULL FOR UPDATE", id).Scan(&locked); err != nil {
		return protocol.Host{}, err
	}
	return scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hosts WHERE id=$1 AND removed_at IS NULL", id))
}
func (a *API) workspace(ctx context.Context) (protocol.Workspace, error) {
	capturedAt := time.Now().UTC()
	rows, err := a.DB.Query(ctx, "SELECT "+columns+" FROM hosts WHERE removed_at IS NULL ORDER BY created_at, id")
	if err != nil {
		return protocol.Workspace{}, err
	}
	defer rows.Close()
	hosts := []protocol.Host{}
	for rows.Next() {
		h, err := scan(rows)
		if err != nil {
			return protocol.Workspace{}, err
		}
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return protocol.Workspace{}, err
	}
	rows.Close()
	machines, err := a.machines(ctx)
	if err != nil {
		return protocol.Workspace{}, err
	}
	matches, err := a.matches(ctx)
	if err != nil {
		return protocol.Workspace{}, err
	}
	teams, err := a.teams(ctx)
	if err != nil {
		return protocol.Workspace{}, err
	}
	tournaments, err := a.tournaments(ctx)
	if err != nil {
		return protocol.Workspace{}, err
	}
	openRooms := make(map[string]bool)
	for _, match := range matches {
		if match.Status == "open" || match.LoadStatus == "pending" || match.CancelStatus == "pending" {
			openRooms[match.HostID] = true
		}
	}
	for i := range hosts {
		hosts[i].Actions.OpenMatch = canOpenMatch(hosts[i]) && !openRooms[hosts[i].ID]
		hosts[i].Actions.Restart = hosts[i].Actions.Restart && !openRooms[hosts[i].ID]
		hosts[i].Actions.StopAndUpdate = hosts[i].Actions.StopAndUpdate && !openRooms[hosts[i].ID]
	}
	hostByID := make(map[string]protocol.Host, len(hosts))
	for _, host := range hosts { hostByID[host.ID] = host }
	for i := range matches {
		host, found := hostByID[matches[i].HostID]
		matches[i].CanReopen = matches[i].CanReopen && found && host.Actions.OpenMatch && hostHasMap(host, matches[i].Map)
	}
	rows, err = a.DB.Query(ctx, `SELECT steam_id, game_name FROM players ORDER BY lower(game_name), steam_id`)
	if err != nil {
		return protocol.Workspace{}, err
	}
	defer rows.Close()
	players := []protocol.Player{}
	for rows.Next() {
		var p protocol.Player
		if err := rows.Scan(&p.SteamID, &p.GameName); err != nil {
			return protocol.Workspace{}, err
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return protocol.Workspace{}, err
	}
	return protocol.Workspace{CapturedAt: capturedAt.Format("2006-01-02T15:04:05.000000000Z"), Hosts: hosts, Matches: matches, Teams: teams, Tournaments: tournaments, Maps: protocol.Maps, Machines: machines, Presets: protocol.Presets, Players: players}, nil
}
func (a *API) list(w http.ResponseWriter, r *http.Request) {
	state, err := a.workspace(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, state)
}

var hostname = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,252}$`)

func validAddress(s string) bool { return net.ParseIP(s) != nil || hostname.MatchString(s) }
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Port    int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if in.Name == "" || len(in.Name) > 80 || !validAddress(in.Address) || in.Port < 1 || in.Port > 65535 {
		fail(w, 400, "Provide a name, hostname/IP, and port between 1 and 65535")
		return
	}
	id := rand.Text()
	token := rand.Text() + rand.Text()
	h, err := scan(a.DB.QueryRow(r.Context(), "INSERT INTO hosts (id,name,address,port,token_hash) VALUES ($1,$2,$3,$4,$5) RETURNING "+columns, id, in.Name, in.Address, in.Port, digest(token)))
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]any{"host": h, "agentToken": token})
}

func (a *API) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Map              *string   `json:"map"`
		State            *string   `json:"desiredState"`
		Revision         *int64    `json:"revision"`
		Whitelist        *[]string `json:"whitelist"`
		Preset           *string   `json:"preset"`
		AllowPlayerRooms *bool     `json:"allowPlayerRooms"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Map == nil && in.State == nil && in.Whitelist == nil && in.Preset == nil && in.AllowPlayerRooms == nil {
		fail(w, 400, "No changes supplied")
		return
	}
	if in.Preset != nil && !protocol.ValidPreset(*in.Preset) {
		fail(w, 400, "Unsupported preset")
		return
	}
	if in.Whitelist != nil {
		if err := protocol.ValidateWhitelist(*in.Whitelist); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	if in.Map != nil && !protocol.ValidInstalledMap(*in.Map) {
		fail(w, 400, "Unsupported map")
		return
	}
	if in.State != nil && *in.State != "running" && *in.State != "stopped" {
		fail(w, 400, "Unsupported state")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	h, err := lockHost(r.Context(), tx, r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Host not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !h.Online {
		fail(w, 409, "Host is offline; connect its agent first")
		return
	}
	if in.Revision == nil || *in.Revision != h.Revision {
		fail(w, 409, "Server state changed; refresh and try again")
		return
	}
	var restartPhase string
	err = tx.QueryRow(r.Context(), "SELECT phase FROM server_restarts WHERE host_id=$1 FOR UPDATE", h.ID).Scan(&restartPhase)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internal(w, err)
		return
	}
	if restartPhase == "stopping" || restartPhase == "starting" || restartPhase == "cancelling" {
		if in.State == nil || *in.State != "stopped" || in.Map != nil || in.Whitelist != nil || in.Preset != nil || in.AllowPlayerRooms != nil {
			fail(w, 409, "Cancel the pending restart before changing this server")
			return
		}
		if _, err = tx.Exec(r.Context(), "UPDATE server_restarts SET phase='cancelling',updated_at=now() WHERE host_id=$1", h.ID); err != nil {
			internal(w, err)
			return
		}
	}
	if in.AllowPlayerRooms != nil {
		if !h.Actions.ChangePlayerRooms {
			fail(w, 409, "Player room permissions are unavailable for this server")
			return
		}
		h.AllowPlayerRooms = *in.AllowPlayerRooms
	}
	if in.Whitelist != nil {
		if !h.Actions.ChangeWhitelist || (in.State != nil && *in.State != "stopped") {
			fail(w, 409, "Stop the server and connect an agent that supports roster editing")
			return
		}
		h.Whitelist = append([]string{}, (*in.Whitelist)...)
		h.Project(time.Now())
	}
	if in.Preset != nil {
		if !h.Actions.ChangePreset || (*in.Preset != "" && h.ObservedState != "stopped") || (in.State != nil && *in.State != "stopped") {
			fail(w, 409, "Stop an installed server and connect an updated agent before changing its preset")
			return
		}
		h.Preset = in.Preset
		h.Project(time.Now())
	}
	if in.Map != nil && !hostHasMap(h, *in.Map) {
		fail(w, 409, "Map is not currently available on this server")
		return
	}
	if in.Map != nil && *in.Map != h.Map {
		if !h.Actions.ChangeMap {
			fail(w, 409, "Stop the server before changing its map")
			return
		}
		h.Map = *in.Map
		h.Project(time.Now())
	}
	if in.State != nil && *in.State == "running" && !hostHasMap(h, h.Map) {
		fail(w, 409, "Map is not currently available on this server")
		return
	}
	if in.State != nil && *in.State != h.DesiredState {
		if (*in.State == "running" && !h.Actions.Start) || (*in.State == "stopped" && !h.Actions.Stop) {
			fail(w, 409, "Action is unavailable in the current server state")
			return
		}
		h.DesiredState = *in.State
	}
	_, err = tx.Exec(r.Context(), `UPDATE hosts SET map=$2,desired_state=$3,whitelist=$4,preset=$5,allow_player_rooms=$6,
		mode_command_status=CASE WHEN $3='stopped' AND mode_command_status='pending' THEN 'cancelled' ELSE mode_command_status END,
		mode_command_error=CASE WHEN $3='stopped' AND mode_command_status='pending' THEN 'Server stopped before MatchZy confirmed the change' ELSE mode_command_error END,
		match_control_status=CASE WHEN $3='stopped' AND match_control_status='pending' THEN 'failed' ELSE match_control_status END,
		match_control_error=CASE WHEN $3='stopped' AND match_control_status='pending' THEN CASE WHEN match_control_action='end' THEN 'Server stopped before MatchZy confirmed that the match ended' ELSE 'Server stopped before MatchZy confirmed the pause state' END ELSE match_control_error END,
		revision=revision+1 WHERE id=$1`, h.ID, h.Map, h.DesiredState, h.Whitelist, h.Preset, h.AllowPlayerRooms)
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	if h.DesiredState == "stopped" && h.MatchControlStatus == "pending" {
		h.MatchControlStatus = "failed"
		if h.MatchControlAction == "end" {
			h.MatchControlError = "Server stopped before MatchZy confirmed that the match ended"
		} else {
			h.MatchControlError = "Server stopped before MatchZy confirmed the pause state"
		}
	}
	h.Revision++
	h.Project(time.Now())
	respond(w, 200, h)
}

func (a *API) heartbeat(w http.ResponseWriter, r *http.Request) {
	var hash string
	err := a.DB.QueryRow(r.Context(), "SELECT token_hash FROM hosts WHERE id=$1 AND machine_id IS NULL", r.PathValue("id")).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "Invalid agent credentials")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(digest(bearer(r))), []byte(hash)) != 1 {
		fail(w, 401, "Invalid agent credentials")
		return
	}
	var in protocol.Report
	if !decodeLimit(w, r, &in, 16384) {
		return
	}
	if !validReport(in) || in.State == "removed" {
		fail(w, 400, "Invalid agent status")
		return
	}
	var desired protocol.Desired
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var workshopMaps []string
	if in.WorkshopMaps != nil {
		workshopMaps = *in.WorkshopMaps
	}
	err = tx.QueryRow(r.Context(), "UPDATE hosts SET observed_state=$2,active_map=$3,driver=$4,error=$5,players=$6,max_players=$7,log_tail=$8,access_status=$9,last_seen=now(),workshop_maps=$10,workshop_maps_seen=CASE WHEN $11::boolean THEN clock_timestamp() ELSE NULL END WHERE id=$1 AND machine_id IS NULL RETURNING desired_state,map,port", r.PathValue("id"), in.State, in.ActiveMap, in.Driver, in.Error, in.Players, in.MaxPlayers, in.LogTail, in.AccessStatus, workshopMaps, in.WorkshopMaps != nil).Scan(&desired.State, &desired.Map, &desired.Port)
	if err != nil {
		internal(w, err)
		return
	}
	h, err := lockHost(r.Context(), tx, r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	if !hostHasMap(h, desired.Map) {
		desired.State = "stopped"
		desired.Map = protocol.Maps[0]
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, desired)
}
