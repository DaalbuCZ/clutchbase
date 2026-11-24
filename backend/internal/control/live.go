package control

import (
	"context"
	"errors"
	"net/http"
	"time"

	"clutchbase/internal/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5"
)

func (a *API) live(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" && a.origin() != "" && origin != a.origin() {
		fail(w, http.StatusForbidden, "Same-origin live connection required")
		return
	}
	options := &websocket.AcceptOptions{}
	if origin := a.origin(); origin != "" {
		options.OriginPatterns = []string{origin}
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1024)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if a.Context != nil {
		stop := context.AfterFunc(a.Context, cancel)
		defer stop()
	}
	authCtx, authCancel := context.WithTimeout(ctx, 5*time.Second)
	var auth struct {
		Token string `json:"token"`
	}
	err = wsjson.Read(authCtx, conn, &auth)
	authCancel()
	if err != nil {
		return
	}
	viewName := r.URL.Query().Get("view")
	accessRequests := viewName == "access-requests" || viewName == "player-access-requests"
	player := viewName == "player" || viewName == "player-history" || viewName == "player-stats" || viewName == "player-leaderboard" || viewName == "player-reports" || viewName == "player-access-requests"
	history := viewName == "history" || viewName == "player-history"
	statistics := viewName == "stats" || viewName == "player-stats"
	leaderboard := viewName == "leaderboard" || viewName == "player-leaderboard"
	reports := viewName == "reports" || viewName == "player-reports"
	adminAccessRequests := viewName == "access-requests"
	cursor := r.URL.Query().Get("before")
	search := r.URL.Query().Get("search")
	if len(cursor) > 64 || len(search) > 160 || (!history && (cursor != "" || search != "")) {
		return
	}
	if !player && (a.AdminToken == "" || !equal(auth.Token, a.AdminToken)) {
		liveUnauthorized(ctx, conn)
		return
	}
	if player {
		sessionCtx, sessionCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := a.session(sessionCtx, r)
		sessionCancel()
		if err != nil {
			if errors.Is(err, errNoSession) {
				liveUnauthorized(ctx, conn)
			}
			return
		}
	}
	ctx = conn.CloseRead(ctx)
	// Each live tab owns a LISTEN connection, separate from the query pool.
	// shortcut: 64 live tabs per API process; use a shared fan-out before larger deployments.
	config := a.DB.Config().ConnConfig.Copy()
	config.ConnectTimeout = 5 * time.Second
	setup, done := context.WithTimeout(ctx, 5*time.Second)
	listener, err := pgx.ConnectConfig(setup, config)
	if err != nil {
		done()
		return
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		_ = listener.Close(closeCtx)
	}()
	_, err = listener.Exec(setup, "LISTEN clutchbase_state")
	var schema string
	if err == nil {
		err = listener.QueryRow(setup, "SELECT current_schema()").Scan(&schema)
	}
	done()
	if err != nil {
		return
	}
	pingAt := time.Now().Add(25 * time.Second)
	var expires time.Time
	send := func() bool {
		readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
		defer readCancel()
		var identity playerIdentity
		if player {
			var err error
			identity, err = a.session(readCtx, r)
			if err != nil {
				if errors.Is(err, errNoSession) {
					liveUnauthorized(ctx, conn)
				}
				return false
			}
		}
		if accessRequests {
			if adminAccessRequests {
				page, err := a.listAccessRequestsData(readCtx)
				if err != nil {
					return false
				}
				expires = a.accessRequestExpiry(readCtx, time.Now())
				return wsjson.Write(readCtx, conn, page) == nil
			}
			page, err := a.accessRequestsForPlayer(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			if !time.Now().Before(identity.ExpiresAt) {
				liveUnauthorized(ctx, conn)
				return false
			}
			expires = identity.ExpiresAt
			return wsjson.Write(readCtx, conn, page) == nil
		}
		if history {
			page, err := a.matchHistory(readCtx, identity.SteamID, cursor, search)
			if err != nil {
				return false
			}
			expires = identity.ExpiresAt
			return wsjson.Write(readCtx, conn, page) == nil
		}
		if statistics {
			page, err := a.playerSummaries(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			if player && !time.Now().Before(identity.ExpiresAt) {
				liveUnauthorized(ctx, conn)
				return false
			}
			expires = identity.ExpiresAt
			return wsjson.Write(readCtx, conn, page) == nil
		}
		if reports {
			page, err := a.matchReports(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			if player && !time.Now().Before(identity.ExpiresAt) {
				liveUnauthorized(ctx, conn)
				return false
			}
			expires = identity.ExpiresAt
			return wsjson.Write(readCtx, conn, page) == nil
		}
		if leaderboard {
			page, err := a.leaderboard(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			if player && !time.Now().Before(identity.ExpiresAt) {
				liveUnauthorized(ctx, conn)
				return false
			}
			expires = identity.ExpiresAt
			return wsjson.Write(readCtx, conn, page) == nil
		}
		state, err := a.workspace(readCtx)
		if err != nil {
			return false
		}
		if player && !time.Now().Before(identity.ExpiresAt) {
			liveUnauthorized(ctx, conn)
			return false
		}
		expires = nextExpiry(state, identity.ExpiresAt, time.Now())
		var payload any = state
		if player {
			view := projectPlayer(state, identity.SteamID)
			view.GameName = identity.GameName
			view.Matches, err = a.playerMatches(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			a.projectPlayerRoomActions(&view, state, identity.SteamID)
			view.Team, err = a.playerTeam(readCtx, identity.SteamID)
			if err != nil {
				return false
			}
			view.Tournaments = projectPlayerTournaments(state.Tournaments, identity.SteamID, view.Team)
			payload = view
		}
		return wsjson.Write(readCtx, conn, payload) == nil
	}
	if !send() {
		return
	}
	for ctx.Err() == nil {
		deadline := pingAt
		if !expires.IsZero() && expires.Before(deadline) {
			deadline = expires
		}
		waitCtx, waitCancel := context.WithDeadline(ctx, deadline)
		notification, err := listener.WaitForNotification(waitCtx)
		waitCancel()
		if err == nil {
			if notification.Payload == schema && !send() {
				return
			}
			continue
		}
		if ctx.Err() != nil || !errors.Is(err, context.DeadlineExceeded) {
			return
		}
		now := time.Now()
		if !expires.IsZero() && !now.Before(expires) {
			if !send() {
				return
			}
		}
		if !now.Before(pingAt) {
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			err = conn.Ping(pingCtx)
			if err == nil {
				err = wsjson.Write(pingCtx, conn, map[string]string{"type": "heartbeat"})
			}
			pingCancel()
			if err != nil {
				return
			}
			pingAt = time.Now().Add(25 * time.Second)
		}
	}
}

func liveUnauthorized(ctx context.Context, c *websocket.Conn) {
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = wsjson.Write(deadline, c, map[string]any{"error": "Sign in again", "code": 401})
}

// Heartbeat expiry is a server event even when no row changes.
func nextExpiry(state protocol.Workspace, sessionExpiry, now time.Time) time.Time {
	var next time.Time
	add := func(at time.Time) {
		if at.After(now) && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	for _, h := range state.Hosts {
		if h.GameBuildCheckedAt != nil && h.GameBuildStatus != "unknown" {
			add(h.GameBuildCheckedAt.Add(2 * time.Hour))
		}
		if h.WorkshopMapsSeen != nil {
			add(h.WorkshopMapsSeen.Add(20 * time.Second))
		}
		if h.LastSeen != nil {
			at := h.LastSeen.Add(20 * time.Second)
			if h.Online && !at.After(now) {
				at = now.Add(time.Millisecond)
			}
			add(at)
		}
	}
	for _, match := range state.Matches {
		if match.LiveScore != nil {
			at := time.UnixMilli(match.LiveScore.ObservedAt).Add(15 * time.Second)
			if !at.After(now) {
				at = now.Add(time.Millisecond)
			}
			add(at)
		}
	}
	for _, m := range state.Machines {
		if m.LastSeen != nil {
			at := m.LastSeen.Add(20 * time.Second)
			if m.Online && !at.After(now) {
				at = now.Add(time.Millisecond)
			}
			add(at)
		}
	}
	if !sessionExpiry.IsZero() {
		add(sessionExpiry)
	}
	return next
}
