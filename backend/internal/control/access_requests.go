package control

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"strings"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

func (a *API) accessRequestExpiry(ctx context.Context, now time.Time) time.Time {
	var at *time.Time
	err := a.DB.QueryRow(ctx, `SELECT MIN(h.last_seen+interval '20 seconds') FROM hosts h WHERE h.last_seen+interval '20 seconds'>$1 AND EXISTS (SELECT 1 FROM access_requests r WHERE r.host_id=h.id AND r.status='pending')`, now).Scan(&at)
	if err != nil || at == nil {
		return time.Time{}
	}
	if !at.After(now) {
		return now.Add(time.Millisecond)
	}
	return *at
}

type playerAccessRequest struct {
	ID       string `json:"id"`
	HostID   string `json:"hostId"`
	HostName string `json:"hostName"`
	Status   string `json:"status"`
	Revision int64  `json:"revision"`
}

type adminAccessRequest struct {
	playerAccessRequest
	SteamID                 string `json:"steamId"`
	GameName                string `json:"gameName"`
	CanApprove              bool   `json:"canApprove"`
	RequiresWhitelistEnable bool   `json:"requiresWhitelistEnable"`
	Blocker                 string `json:"blocker"`
}

type accessRequestRow struct {
	playerAccessRequest
	SteamID   string
	GameName  string
	Whitelist []string
}

func (a *API) playerAccessRequests(w http.ResponseWriter, r *http.Request) {
	p, err := a.session(r.Context(), r)
	if errors.Is(err, errNoSession) {
		fail(w, http.StatusUnauthorized, "Sign in with Steam")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	requests, err := a.accessRequestsForPlayer(r.Context(), p.SteamID)
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, requests)
}

func (a *API) createPlayerAccessRequest(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		HostID string `json:"hostId"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.HostID = strings.TrimSpace(in.HostID)
	if in.HostID == "" || len(in.HostID) > 128 {
		fail(w, http.StatusBadRequest, "Provide a server share link")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// The explicit share link authorizes exposing only the display name. Locking
	// the row makes a concurrent soft removal resolve consistently.
	var hostName string
	err = tx.QueryRow(r.Context(), `SELECT name FROM hosts WHERE id=$1 AND removed_at IS NULL FOR SHARE`, in.HostID).Scan(&hostName)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	requestID := rand.Text()
	if _, err = tx.Exec(r.Context(), `INSERT INTO access_requests(id,host_id,steam_id) VALUES($1,$2,$3)
		ON CONFLICT(host_id,steam_id) DO UPDATE SET status=CASE WHEN access_requests.status='rejected' OR (access_requests.status='approved' AND NOT (access_requests.steam_id=ANY(COALESCE((SELECT whitelist FROM hosts WHERE hosts.id=access_requests.host_id),ARRAY[]::text[])))) THEN 'pending' ELSE access_requests.status END,
		revision=CASE WHEN access_requests.status='rejected' OR (access_requests.status='approved' AND NOT (access_requests.steam_id=ANY(COALESCE((SELECT whitelist FROM hosts WHERE hosts.id=access_requests.host_id),ARRAY[]::text[])))) THEN access_requests.revision+1 ELSE access_requests.revision END,
		updated_at=CASE WHEN access_requests.status='rejected' OR (access_requests.status='approved' AND NOT (access_requests.steam_id=ANY(COALESCE((SELECT whitelist FROM hosts WHERE hosts.id=access_requests.host_id),ARRAY[]::text[])))) THEN now() ELSE access_requests.updated_at END`, requestID, in.HostID, p.SteamID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT id FROM access_requests WHERE host_id=$1 AND steam_id=$2`, in.HostID, p.SteamID).Scan(&requestID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	request, err := a.accessRequestByID(r.Context(), requestID, false)
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, request)
}

func (a *API) accessRequestsForPlayer(ctx context.Context, steamID string) ([]playerAccessRequest, error) {
	rows, err := a.DB.Query(ctx, `SELECT r.id,r.host_id,h.name,r.status,r.revision,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id WHERE r.steam_id=$1 ORDER BY r.created_at,r.id`, steamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []playerAccessRequest{}
	for rows.Next() {
		var request playerAccessRequest
		var whitelist []string
		if err := rows.Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &whitelist); err != nil {
			return nil, err
		}
		if request.Status == "approved" && !containsSteamID(whitelist, steamID) {
			request.Status = "revoked"
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func (a *API) accessRequestByID(ctx context.Context, id string, admin bool) (any, error) {
	var request accessRequestRow
	err := a.DB.QueryRow(ctx, `SELECT r.id,r.host_id,h.name,r.status,r.revision,r.steam_id,p.game_name,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id JOIN players p ON p.steam_id=r.steam_id WHERE r.id=$1`, id).Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &request.SteamID, &request.GameName, &request.Whitelist)
	if err != nil {
		return nil, err
	}
	if !admin {
		if request.Status == "approved" && !containsSteamID(request.Whitelist, request.SteamID) {
			request.Status = "revoked"
		}
		return request.playerAccessRequest, nil
	}
	state, err := a.workspace(ctx)
	if err != nil {
		return nil, err
	}
	return decorateAdminAccessRequest(request, state.Hosts), nil
}

func (a *API) listAccessRequests(w http.ResponseWriter, r *http.Request) {
	requests, err := a.listAccessRequestsData(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, requests)
}

func (a *API) listAccessRequestsData(ctx context.Context) ([]adminAccessRequest, error) {
	state, err := a.workspace(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := a.DB.Query(ctx, `SELECT r.id,r.host_id,h.name,r.status,r.revision,r.steam_id,p.game_name,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id JOIN players p ON p.steam_id=r.steam_id ORDER BY r.created_at,r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := []adminAccessRequest{}
	for rows.Next() {
		var request accessRequestRow
		if err = rows.Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &request.SteamID, &request.GameName, &request.Whitelist); err != nil {
			return nil, err
		}
		requests = append(requests, decorateAdminAccessRequest(request, state.Hosts))
	}
	return requests, rows.Err()
}

func decorateAdminAccessRequest(request accessRequestRow, hosts []protocol.Host) adminAccessRequest {
	out := adminAccessRequest{playerAccessRequest: request.playerAccessRequest, SteamID: request.SteamID, GameName: request.GameName}
	for _, host := range hosts {
		if host.ID == request.HostID {
			out.RequiresWhitelistEnable = len(host.Whitelist) == 0
			if request.Status == "approved" && !containsSteamID(host.Whitelist, request.SteamID) {
				out.Status = "revoked"
				out.Blocker = "Access is no longer on the server roster."
				return out
			}
			if request.Status != "pending" {
				out.Blocker = "Request is no longer pending."
				return out
			}
			out.CanApprove, out.Blocker = accessApprovalState(host, request.SteamID)
			return out
		}
	}
	out.Blocker = "Server was removed."
	return out
}

func containsSteamID(roster []string, steamID string) bool {
	for _, id := range roster {
		if id == steamID {
			return true
		}
	}
	return false
}

func accessApprovalState(h protocol.Host, steamID string) (bool, string) {
	if !h.Online {
		return false, "Server agent is offline."
	}
	if h.DesiredState != "stopped" || h.ObservedState != "stopped" {
		return false, "Stop the server before approving access."
	}
	if h.Removing || h.Error != "" || h.GameUpdateStatus == "pending" || h.ReporterUpdateStatus == "pending" || h.ModeCommandStatus == "pending" || h.MatchControlStatus == "pending" || h.RestartPhase == "stopping" || h.RestartPhase == "starting" || h.RestartPhase == "cancelling" {
		return false, "Server maintenance is in progress."
	}
	if !h.CanWhitelist || h.MachineID == "" {
		return false, "The connected agent does not support roster editing."
	}
	if !h.Actions.ChangeWhitelist {
		return false, "Server roster editing is temporarily unavailable."
	}
	next := append([]string(nil), h.Whitelist...)
	for _, id := range next {
		if id == steamID {
			return true, ""
		}
	}
	if err := protocol.ValidateWhitelist(append(next, steamID)); err != nil {
		return false, "The server SteamID roster is full."
	}
	return true, ""
}

func (a *API) updateAccessRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision        int64  `json:"revision"`
		Action          string `json:"action"`
		EnableWhitelist *bool  `json:"enableWhitelist"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Revision < 1 || (in.Action != "approve" && in.Action != "reject") {
		fail(w, http.StatusBadRequest, "Provide a request revision and approve or reject action")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var request accessRequestRow
	// Read the host ID first. Approval takes the host lock before the request
	// lock, matching player submission's host-then-request order.
	err = tx.QueryRow(r.Context(), `SELECT r.id,r.host_id,h.name,r.status,r.revision,r.steam_id,p.game_name,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id JOIN players p ON p.steam_id=r.steam_id WHERE r.id=$1`, r.PathValue("id")).Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &request.SteamID, &request.GameName, &request.Whitelist)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Access request not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if in.Action == "approve" {
		h, hostErr := lockHost(r.Context(), tx, request.HostID)
		if errors.Is(hostErr, pgx.ErrNoRows) {
			fail(w, http.StatusConflict, "Server was removed")
			return
		}
		if hostErr != nil {
			internal(w, hostErr)
			return
		}
		if err = tx.QueryRow(r.Context(), `SELECT r.id,r.host_id,h.name,r.status,r.revision,r.steam_id,p.game_name,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id JOIN players p ON p.steam_id=r.steam_id WHERE r.id=$1 FOR UPDATE OF r`, r.PathValue("id")).Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &request.SteamID, &request.GameName, &request.Whitelist); err != nil {
			internal(w, err)
			return
		}
		if request.Revision != in.Revision {
			fail(w, http.StatusConflict, "Access request changed; refresh and try again")
			return
		}
		if request.Status != "pending" {
			fail(w, http.StatusConflict, "Access request is no longer pending")
			return
		}
		// Empty/nil rosters disable enforcement. Enabling it is an explicit
		// administrator decision, checked only after taking the host lock so a
		// stale snapshot cannot silently replace an existing roster.
		if len(h.Whitelist) == 0 && (in.EnableWhitelist == nil || !*in.EnableWhitelist) {
			fail(w, http.StatusConflict, "Explicitly enable the whitelist before approving access")
			return
		}
		canApprove, blocker := accessApprovalState(h, request.SteamID)
		if !canApprove {
			fail(w, http.StatusConflict, blocker)
			return
		}
		next := append([]string(nil), h.Whitelist...)
		already := false
		for _, id := range next {
			if id == request.SteamID {
				already = true
				break
			}
		}
		if !already {
			next = append(next, request.SteamID)
		}
		if err = protocol.ValidateWhitelist(next); err != nil {
			fail(w, http.StatusConflict, "The server SteamID roster is full")
			return
		}
		// This is intent only. The agent must acknowledge the hash before the
		// canonical host projection considers the roster applied or joinable.
		if _, err = tx.Exec(r.Context(), "UPDATE hosts SET whitelist=$2,revision=revision+1 WHERE id=$1", h.ID, next); err != nil {
			internal(w, err)
			return
		}
	}
	if in.Action == "reject" {
		if err = tx.QueryRow(r.Context(), `SELECT r.id,r.host_id,h.name,r.status,r.revision,r.steam_id,p.game_name,h.whitelist FROM access_requests r JOIN hosts h ON h.id=r.host_id JOIN players p ON p.steam_id=r.steam_id WHERE r.id=$1 FOR UPDATE OF r`, r.PathValue("id")).Scan(&request.ID, &request.HostID, &request.HostName, &request.Status, &request.Revision, &request.SteamID, &request.GameName, &request.Whitelist); err != nil {
			internal(w, err)
			return
		}
		if request.Revision != in.Revision {
			fail(w, http.StatusConflict, "Access request changed; refresh and try again")
			return
		}
		if request.Status != "pending" {
			fail(w, http.StatusConflict, "Access request is no longer pending")
			return
		}
	}
	status := "approved"
	if in.Action == "reject" {
		status = "rejected"
	}
	if _, err = tx.Exec(r.Context(), "UPDATE access_requests SET status=$2,revision=revision+1,updated_at=now() WHERE id=$1", request.ID, status); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	result, err := a.accessRequestByID(r.Context(), request.ID, true)
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, result)
}
