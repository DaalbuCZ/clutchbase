package control

import (
	"context"
	"errors"
	"net/http"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

// The caller holds the host and room locks; re-read membership after acquiring them.
func (a *API) playerRoomPermission(ctx context.Context, tx pgx.Tx, room, steam string, forLoad bool) (bool, error) {
	var hostID, owner, status, load string
	var joined, tournament bool
	err := tx.QueryRow(ctx, `SELECT m.host_id,COALESCE(m.created_by_steam_id,''),m.status,m.load_status,EXISTS(SELECT 1 FROM match_room_players WHERE match_id=m.id AND steam_id=$2),EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=m.id) FROM match_rooms m WHERE m.id=$1`, room, steam).Scan(&hostID, &owner, &status, &load, &joined, &tournament)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	h, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hosts WHERE id=$1 AND removed_at IS NULL", hostID))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rostered := false
	for _, id := range h.Whitelist {
		if id == steam {
			rostered = true
			break
		}
	}
	return owner == steam && joined && rostered && h.AllowPlayerRooms && status == "open" && load == "" && !tournament && (!forLoad || playerJoinBlocker(h) == ""), nil
}

func (a *API) playerCloseMatch(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF h`, r.PathValue("id")).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Room unavailable for player management")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Room unavailable for player management")
			return
		}
		internal(w, err)
		return
	}
	allowed, err := a.playerRoomPermission(ctx, tx, id, player.SteamID, false)
	if err != nil {
		internal(w, err)
		return
	}
	if !allowed {
		fail(w, 404, "Room unavailable for player management")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE match_rooms SET status='closed',closed_at=now() WHERE id=$1`, id); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) projectPlayerRoomActions(view *playerWorkspace, state protocol.Workspace, steam string) {
	hosts := make(map[string]protocol.Host, len(state.Hosts))
	for _, h := range state.Hosts {
		hosts[h.ID] = h
	}
	matches := make(map[string]protocol.Match, len(state.Matches))
	for _, m := range state.Matches {
		matches[m.ID] = m
	}
	for i := range view.Matches {
		m := &view.Matches[i]
		h, found := hosts[m.HostID]
		admin, foundMatch := matches[m.ID]
		if !found || !foundMatch {
			continue
		}
		if m.MyRoom && m.Joined && h.AllowPlayerRooms && admin.CanReopen && playerJoinBlocker(h) == "" {
			m.Actions.Reopen = true
			m.SettingsRevision = admin.SettingsRevision
		}
		if !m.MyRoom || !m.Joined || !h.AllowPlayerRooms {
			if m.Status == "open" && m.LoadStatus == "" && len(m.LoadBlockers) == 0 {
				m.LoadBlockers = append(m.LoadBlockers, "load_permission_required")
			}
			continue
		}
		rostered := false
		for _, id := range h.Whitelist {
			if id == steam {
				rostered = true
				break
			}
		}
		if !rostered {
			continue
		}
		editable := m.Status == "open" && m.LoadStatus == ""
		m.Actions.Close = editable && admin.CanEdit
		m.Actions.Edit = editable && admin.CanEdit
		if editable && admin.CanShuffle && len(m.Participants) == len(admin.Participants) {
			roster := make([]shufflePlayer, 0, len(admin.Participants))
			consistent := true
			for j, p := range admin.Participants {
				visible := m.Participants[j]
				if p.GameName != visible.GameName || p.Side != visible.Side || p.Ready != visible.Ready || p.SteamID == "" {
					consistent = false
					break
				}
				roster = append(roster, shufflePlayer{SteamID: p.SteamID, Side: p.Side, Ready: p.Ready})
			}
			if consistent {
				m.Actions.Shuffle = true
				m.RosterFingerprint = a.rosterFingerprint(m.ID, roster)
				for j, p := range admin.Participants {
					if p.SteamID != steam {
						m.Participants[j].RemovalToken = a.participantRemovalToken(m.ID, p.SteamID)
					}
				}
			}
		}
		if m.Actions.Edit {
			m.SettingsRevision = admin.SettingsRevision
		}
		m.Actions.Load = editable && m.Players > 0 && m.Ready == m.Players && m.CTPlayers > 0 && m.TPlayers > 0 && admin.CanLoad && playerJoinBlocker(h) == ""
		if !m.Actions.Load && len(m.LoadBlockers) == 0 {
			if playerJoinBlocker(h) != "" {
				m.LoadBlockers = append(m.LoadBlockers, "player_access_unavailable")
			} else {
				// The player endpoint additionally requires room-owner permission;
				// do not expose another player's identity in the explanation.
				m.LoadBlockers = append(m.LoadBlockers, "load_permission_required")
			}
		}
	}
}
