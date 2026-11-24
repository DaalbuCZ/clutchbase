package control

import (
	"errors"
	"net/http"
	"strings"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

func (a *API) editMatchSettings(w http.ResponseWriter, r *http.Request) {
	a.editMatchSettingsFor(w, r, "")
}

func (a *API) playerEditMatchSettings(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	a.editMatchSettingsFor(w, r, player.SteamID)
}

func (a *API) editMatchSettingsFor(w http.ResponseWriter, r *http.Request, steam string) {
	var in struct {
		Title            string `json:"title"`
		Map              string `json:"map"`
		Overtime         *bool  `json:"overtime"`
		ManualReady      *bool  `json:"manualReady"`
		PlayersPerSide   int    `json:"playersPerSide"`
		SettingsRevision *int64 `json:"settingsRevision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Overtime == nil {
		fail(w, 400, "Choose whether overtime is enabled")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if len(in.Title) < 1 || len(in.Title) > 80 || !protocol.ValidInstalledMap(in.Map) || in.PlayersPerSide < 1 || in.PlayersPerSide > 5 {
		fail(w, 400, "Choose a title, available map, and room size from 1v1 to 5v5")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var hostID string
	if err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF h`, r.PathValue("id")).Scan(&hostID); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	} else if err != nil {
		internal(w, err)
		return
	}
	h, err := lockHost(ctx, tx, hostID)
	if err != nil {
		internal(w, err)
		return
	}
	if steam != "" {
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
		err = tx.QueryRow(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "Room unavailable for player management")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		allowed, err := a.playerRoomPermission(ctx, tx, id, steam, false)
		if err != nil {
			internal(w, err)
			return
		}
		if !allowed {
			fail(w, 404, "Room unavailable for player management")
			return
		}
	}
	var current protocol.Match
	var tournamentLinked bool
	err = tx.QueryRow(ctx, `SELECT m.title,m.map,m.overtime,m.players_per_side,m.manual_ready,m.status,m.load_status,m.settings_revision,EXISTS(SELECT 1 FROM tournament_matches WHERE room_id=m.id)
		FROM match_rooms m JOIN hosts h ON h.id=m.host_id
		WHERE m.id=$1 AND h.removed_at IS NULL FOR UPDATE OF m`, r.PathValue("id")).Scan(&current.Title, &current.Map, &current.Overtime, &current.PlayersPerSide, &current.ManualReady, &current.Status, &current.LoadStatus, &current.SettingsRevision, &tournamentLinked)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match room not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if current.Status != "open" || current.LoadStatus != "" {
		fail(w, 409, "Settings are locked after a load request or room closure")
		return
	}
	if tournamentLinked {
		fail(w, 409, "Tournament room settings are fixed by the veto and tournament format")
		return
	}
	if in.SettingsRevision == nil || *in.SettingsRevision != current.SettingsRevision {
		fail(w, 409, "Room settings changed; close and reopen the editor")
		return
	}
	if !hostHasMap(h, in.Map) {
		fail(w, 409, "Map is not currently available on this server")
		return
	}
	var ct, t int
	if err = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE side='ct'),count(*) FILTER(WHERE side='t') FROM match_room_players WHERE match_id=$1`, r.PathValue("id")).Scan(&ct, &t); err != nil {
		internal(w, err)
		return
	}
	if ct > in.PlayersPerSide || t > in.PlayersPerSide {
		fail(w, 409, "The selected room size cannot hold the joined players")
		return
	}
	manualReady := current.ManualReady
	if in.ManualReady != nil {
		manualReady = *in.ManualReady
	}
	resetReady := in.Map != current.Map || in.PlayersPerSide != current.PlayersPerSide || *in.Overtime != current.Overtime || manualReady != current.ManualReady
	if resetReady {
		if _, err = tx.Exec(ctx, "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready=true", r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
	}
	if resetReady || in.Title != current.Title {
		if _, err = tx.Exec(ctx, `UPDATE match_rooms SET title=$2,map=$3,players_per_side=$4,overtime=$5,manual_ready=$6,settings_revision=settings_revision+1 WHERE id=$1`, r.PathValue("id"), in.Title, in.Map, in.PlayersPerSide, *in.Overtime, manualReady); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]bool{"readinessReset": resetReady})
}
