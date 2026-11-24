package control

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

const steamEndpoint = "https://steamcommunity.com/openid/login"
const openIDNamespace = "http://specs.openid.net/auth/2.0"
const playerCookie = "clutchbase-player"
const loginCookie = "clutchbase-steam-state"

var errNoSession = errors.New("player session required")

type playerIdentity struct {
	SteamID   string    `json:"steamId"`
	GameName  string    `json:"gameName"`
	ExpiresAt time.Time `json:"-"`
}

type playerServer struct {
	AvailableMaps       []string `json:"availableMaps"`
	GameBuildStatus     string   `json:"gameBuildStatus"`
	SupportsManualReady bool     `json:"supportsManualReady"`
	MatchPhase          string   `json:"matchPhase"`
	Preset              *string  `json:"preset"`
	ActivePreset        string   `json:"activePreset"`
	PresetStatus        string   `json:"presetStatus"`
	Simulated           bool     `json:"simulated"`
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Address             string   `json:"address"`
	Port                int      `json:"port"`
	Map                 string   `json:"map"`
	Status              string   `json:"status"`
	Players             *int     `json:"players"`
	MaxPlayers          *int     `json:"maxPlayers"`
	CanJoin             bool     `json:"canJoin"`
	CanOpenMatch        bool     `json:"canOpenMatch"`
	JoinBlocker         string   `json:"joinBlocker"`
}

type playerWorkspace struct {
	Maps        []string              `json:"maps"`
	CapturedAt  string                `json:"capturedAt"`
	SteamID     string                `json:"steamId"`
	GameName    string                `json:"gameName"`
	Servers     []playerServer        `json:"servers"`
	Matches     []playerMatch         `json:"matches"`
	Team        *protocol.Team        `json:"team"`
	Tournaments []protocol.Tournament `json:"tournaments"`
}

func (a *API) origin() string {
	u, err := url.Parse(a.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return ""
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))) {
		return ""
	}
	return strings.TrimSuffix(a.Origin, "/")
}

func (a *API) setPlayerCookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/api", HttpOnly: true, Secure: strings.HasPrefix(a.origin(), "https://"), SameSite: http.SameSiteLaxMode, MaxAge: age})
}

func (a *API) session(ctx context.Context, r *http.Request) (playerIdentity, error) {
	cookie, err := r.Cookie(playerCookie)
	if err != nil || len(cookie.Value) != 52 {
		return playerIdentity{}, errNoSession
	}
	var p playerIdentity
	err = a.DB.QueryRow(ctx, `SELECT s.steam_id,p.game_name,s.expires_at FROM player_sessions s
		JOIN players p ON p.steam_id=s.steam_id WHERE s.token_hash=$1 AND s.expires_at>now()`, digest(cookie.Value)).Scan(&p.SteamID, &p.GameName, &p.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return playerIdentity{}, errNoSession
	}
	return p, err
}

func (a *API) playerSession(w http.ResponseWriter, r *http.Request) {
	p, err := a.session(r.Context(), r)
	if errors.Is(err, errNoSession) {
		respond(w, 200, map[string]any{"steamId": nil, "steamEnabled": a.origin() != "", "gameName": ""})
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"steamId": p.SteamID, "steamEnabled": a.origin() != "", "gameName": p.GameName})
}

func (a *API) updatePlayerProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		GameName string `json:"gameName"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.GameName = strings.TrimSpace(in.GameName)
	if !validGameName(in.GameName) {
		fail(w, 400, "Choose an in-game name from 1 to 32 characters without line breaks")
		return
	}
	if _, err := a.DB.Exec(r.Context(), "UPDATE players SET game_name=$2 WHERE steam_id=$1", p.SteamID, in.GameName); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]string{"gameName": in.GameName})
}

func validGameName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 32 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (a *API) playerLogout(w http.ResponseWriter, r *http.Request) {
	// The cookie is not sufficient authorization for a cross-site mutation.
	if a.origin() == "" || r.Header.Get("Origin") != a.origin() {
		fail(w, 403, "Same-origin request required")
		return
	}
	if cookie, err := r.Cookie(playerCookie); err == nil {
		if _, err := a.DB.Exec(r.Context(), "DELETE FROM player_sessions WHERE token_hash=$1", digest(cookie.Value)); err != nil {
			internal(w, err)
			return
		}
	}
	a.setPlayerCookie(w, playerCookie, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) playerState(w http.ResponseWriter, r *http.Request) {
	p, err := a.session(r.Context(), r)
	if errors.Is(err, errNoSession) {
		fail(w, 401, "Sign in with Steam")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	state, err := a.workspace(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	out := projectPlayer(state, p.SteamID)
	out.GameName = p.GameName
	out.Matches, err = a.playerMatches(r.Context(), p.SteamID)
	if err != nil {
		internal(w, err)
		return
	}
	a.projectPlayerRoomActions(&out, state, p.SteamID)
	team, err := a.playerTeam(r.Context(), p.SteamID)
	if err != nil {
		internal(w, err)
		return
	}
	out.Team = team
	out.Tournaments = projectPlayerTournaments(state.Tournaments, p.SteamID, team)
	respond(w, 200, out)
}

func playerJoinBlocker(h protocol.Host) string {
	switch {
	case h.Driver == "simulated":
		return "Simulation does not accept players."
	case h.Driver != "managed":
		return "Verified player access is unavailable on this server."
	case !h.Online:
		return "Server agent is offline."
	case h.Status == "starting":
		return "CS2 is starting."
	case h.Status == "stopping":
		return "CS2 is stopping."
	case h.Status == "stopped" || h.Status == "uninstalled":
		return "Server is stopped."
	case h.Status == "error":
		return "Server needs attention from the host."
	case h.Status != "running":
		return "Server is not ready."
	case !h.WhitelistApplied:
		return "SteamID roster is waiting for host confirmation."
	case h.AccessStatus == "blocked":
		return "SteamID access verification failed; ask the host."
	case h.AccessStatus == "pending":
		return "SteamID access verification is pending."
	case h.AccessStatus != "verified":
		return "SteamID access has not been verified."
	case h.Preset != nil && !h.PresetApplied:
		return "Startup preset is waiting for host confirmation."
	case h.Preset != nil && *h.Preset != "" && h.PresetStatus != "verified":
		return "Startup preset is not verified."
	default:
		return ""
	}
}

func projectPlayer(state protocol.Workspace, steamID string) playerWorkspace {
	out := playerWorkspace{CapturedAt: state.CapturedAt, SteamID: steamID, Maps: state.Maps, Servers: []playerServer{}, Tournaments: []protocol.Tournament{}}
	for _, h := range state.Hosts {
		allowed := false
		for _, id := range h.Whitelist {
			if id == steamID {
				allowed = true
				break
			}
		}
		if !allowed || h.Removing {
			continue
		}
		displayMap := h.Map
		if h.Online && h.DesiredState == "running" && (h.ObservedState == "running" || h.ObservedState == "starting") {
			displayMap = ""
			if h.CanObserveMap || h.Driver == "simulated" {
				displayMap = h.ActiveMap
			}
		}
		joinBlocker := playerJoinBlocker(h)
		out.Servers = append(out.Servers, playerServer{AvailableMaps: h.AvailableMaps, CanOpenMatch: h.AllowPlayerRooms && h.Actions.OpenMatch && joinBlocker == "", GameBuildStatus: h.GameBuildStatus, SupportsManualReady: h.SupportsManualReady, MatchPhase: h.MatchPhase, Preset: h.Preset, ActivePreset: h.ActivePreset, PresetStatus: h.PresetStatus, Simulated: h.Driver == "simulated", ID: h.ID, Name: h.Name, Address: h.Address, Port: h.Port, Map: displayMap, Status: h.Status, Players: h.Players, MaxPlayers: h.MaxPlayers, CanJoin: joinBlocker == "", JoinBlocker: joinBlocker})
	}
	return out
}

func (a *API) playerTeam(ctx context.Context, steamID string) (*protocol.Team, error) {
	teams, err := a.teams(ctx)
	if err != nil {
		return nil, err
	}
	for i := range teams {
		for _, member := range teams[i].Members {
			if member.SteamID == steamID {
				team := &teams[i]
				captain := team.CaptainSteamID == steamID
				team.Actions = protocol.TeamActions{Invite: captain && !team.RosterLocked && len(team.Members) < 5, Leave: !captain && !team.RosterLocked, RemoveMember: captain && !team.RosterLocked && len(team.Members) > 1, TransferCaptain: captain && !team.RosterLocked && len(team.Members) > 1, Disband: captain && !team.Registered}
				return team, nil
			}
		}
	}
	return nil, nil
}

func projectPlayerTournaments(all []protocol.Tournament, steamID string, team *protocol.Team) []protocol.Tournament {
	out := make([]protocol.Tournament, 0, len(all))
	teamID := ""
	captain := false
	if team != nil {
		teamID = team.ID
		captain = team.CaptainSteamID == steamID
	}
	for _, t := range all {
		copy := t
		copy.CanCancel = false
		copy.Entries = append([]protocol.TournamentEntry(nil), t.Entries...)
		copy.Matches = append([]protocol.TournamentMatch(nil), t.Matches...)
		for _, entry := range t.Entries {
			if entry.TeamID == teamID {
				copy.MyEntryStatus = entry.Status
			}
		}
		for i := range copy.Matches {
			match := &copy.Matches[i]
			match.CanSchedule = false
			match.CanCorrectWinner = false
			match.WinnerHistory = nil
			participant := false
			for _, id := range match.DisputeParticipants {
				if id == steamID {
					participant = true
					break
				}
			}
			if !participant {
				match.Dispute = nil
			}
			match.CanDispute = participant && match.DisputeEligible && match.Dispute == nil
			match.CanVeto = teamID != "" && t.Status == "active" && match.Status == "pending" && match.RoomID == "" && match.VetoStep < 7 && match.VetoTurnTeamID == teamID
		}
		copy.MyTeamID = teamID
		if team != nil {
			copy.MyTeamRevision = team.Revision
		}
		copy.CanRegisterTeam = captain && team != nil && t.Status == "open" && len(t.Entries) < 64 && copy.MyEntryStatus == "" && len(team.Members) >= t.PlayersPerSide
		copy.CanWithdrawTeam = captain && (t.Status == "open" || t.Status == "checkin") && len(t.Matches) == 0 && (copy.MyEntryStatus == "registered" || copy.MyEntryStatus == "checked_in")
		copy.CanCheckInTeam = captain && team != nil && t.Status == "checkin" && copy.MyEntryStatus == "registered" && len(team.Members) >= t.PlayersPerSide
		out = append(out, copy)
	}
	return out
}

// Accept public navigation fragments only; never a URL, host or management path.
func playerReturnPath(fragment string) string {
	if len(fragment) > 512 {
		return "/play"
	}
	view, query, _ := strings.Cut(fragment, "?")
	key := ""
	switch view {
	case "#servers", "#teams", "#history", "#stats", "#profile", "#schedule", "#leaderboard", "#reports":
	case "#matches":
		key = "room"
	case "#tournaments":
		key = "tournament"
	case "#access":
		key = "server"
	default:
		return "/play"
	}
	if query == "" {
		return "/play" + view
	}
	values, err := url.ParseQuery(query)
	if err != nil || key == "" || len(values) != 1 || len(values[key]) != 1 {
		return "/play"
	}
	id := values.Get(key)
	if id == "" || len(id) > 128 || strings.IndexFunc(id, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return "/play"
	}
	return "/play" + view + "?" + values.Encode()
}

func (a *API) steamStart(w http.ResponseWriter, r *http.Request) {
	origin := a.origin()
	if origin == "" {
		fail(w, 503, "Steam sign-in requires a configured application origin")
		return
	}
	state := rand.Text() + rand.Text()
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "DELETE FROM steam_logins WHERE expires_at<=now()"); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "DELETE FROM player_sessions WHERE expires_at<=now()"); err != nil {
		internal(w, err)
		return
	}
	returnPath := "/play"
	if destinations := r.URL.Query()["returnTo"]; len(destinations) == 1 {
		returnPath = playerReturnPath(destinations[0])
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO steam_logins(state_hash,expires_at,return_path) VALUES($1,now()+interval '10 minutes',$2)", digest(state), returnPath); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	a.setPlayerCookie(w, loginCookie, state, 600)
	values := url.Values{
		"openid.ns": {openIDNamespace}, "openid.mode": {"checkid_setup"},
		"openid.identity":   {"http://specs.openid.net/auth/2.0/identifier_select"},
		"openid.claimed_id": {"http://specs.openid.net/auth/2.0/identifier_select"},
		"openid.realm":      {origin + "/"}, "openid.return_to": {origin + "/api/auth/steam/callback?state=" + state},
	}
	http.Redirect(w, r, steamEndpoint+"?"+values.Encode(), http.StatusFound)
}

func (a *API) steamCallback(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 8192 {
		fail(w, 400, "Invalid Steam response")
		return
	}
	for key, vs := range values {
		if (strings.HasPrefix(key, "openid.") || key == "state") && len(vs) != 1 {
			fail(w, 400, "Invalid Steam response")
			return
		}
	}
	state := values.Get("state")
	cookie, err := r.Cookie(loginCookie)
	if err != nil || len(state) != 52 || !equal(state, cookie.Value) {
		fail(w, 400, "Steam sign-in expired; try again")
		return
	}
	var exists bool
	if err := a.DB.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM steam_logins WHERE state_hash=$1 AND expires_at>now())", digest(state)).Scan(&exists); err != nil {
		internal(w, err)
		return
	}
	if !exists {
		fail(w, 400, "Steam sign-in expired; try again")
		return
	}
	steamID, err := a.verifySteam(r.Context(), values, a.origin()+"/api/auth/steam/callback?state="+state)
	if err != nil {
		fail(w, 401, "Steam identity could not be verified; try again")
		return
	}
	token := rand.Text() + rand.Text()
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var returnPath string
	err = tx.QueryRow(r.Context(), "DELETE FROM steam_logins WHERE state_hash=$1 AND expires_at>now() RETURNING return_path", digest(state)).Scan(&returnPath)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 400, "Steam sign-in already used or expired")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO players(steam_id) VALUES($1) ON CONFLICT DO NOTHING", steamID); err != nil {
		internal(w, err)
		return
	}
	if old, err := r.Cookie(playerCookie); err == nil {
		if _, err = tx.Exec(r.Context(), "DELETE FROM player_sessions WHERE token_hash=$1", digest(old.Value)); err != nil {
			internal(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), "INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '24 hours')", digest(token), steamID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	a.setPlayerCookie(w, loginCookie, "", -1)
	a.setPlayerCookie(w, playerCookie, token, 86400)
	http.Redirect(w, r, a.origin()+returnPath, http.StatusSeeOther)
}

func (a *API) verifySteam(ctx context.Context, v url.Values, returnTo string) (string, error) {
	invalid := errors.New("invalid Steam assertion")
	if v.Get("openid.sig") == "" || v.Get("openid.assoc_handle") == "" || a.origin() == "" || v.Get("openid.ns") != openIDNamespace || v.Get("openid.mode") != "id_res" || v.Get("openid.op_endpoint") != steamEndpoint || v.Get("openid.return_to") != returnTo {
		return "", invalid
	}
	identity := v.Get("openid.claimed_id")
	if identity != v.Get("openid.identity") {
		return "", invalid
	}
	// Steam has documented both schemes for claimed IDs; verification still uses HTTPS.
	id, ok := strings.CutPrefix(identity, "https://steamcommunity.com/openid/id/")
	if !ok {
		id, ok = strings.CutPrefix(identity, "http://steamcommunity.com/openid/id/")
	}
	if !ok || protocol.ValidateWhitelist([]string{id}) != nil {
		return "", invalid
	}
	signed := map[string]bool{}
	for _, key := range strings.Split(v.Get("openid.signed"), ",") {
		signed[key] = true
	}
	for _, key := range []string{"op_endpoint", "claimed_id", "identity", "return_to", "response_nonce", "assoc_handle"} {
		if !signed[key] {
			return "", invalid
		}
	}
	nonce := v.Get("openid.response_nonce")
	if len(nonce) < 21 || len(nonce) > 256 {
		return "", invalid
	}
	when, err := time.Parse(time.RFC3339, nonce[:20])
	if err != nil || time.Since(when) > 10*time.Minute || time.Until(when) > time.Minute {
		return "", invalid
	}
	form := url.Values{}
	for key, values := range v {
		if strings.HasPrefix(key, "openid.") {
			form[key] = values
		}
	}
	form.Set("openid.mode", "check_authentication")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, steamEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", invalid
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := a.SteamClient
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(req)
	if err != nil {
		return "", invalid
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 || response.StatusCode != 200 {
		return "", invalid
	}
	valid := false
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && key == "is_valid" {
			if valid || value != "true" {
				return "", invalid
			}
			valid = true
		}
	}
	if !valid {
		return "", invalid
	}
	return id, nil
}
