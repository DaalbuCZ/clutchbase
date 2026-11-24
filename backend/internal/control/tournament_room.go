package control

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (a *API) openTournamentMatchRoom(w http.ResponseWriter, r *http.Request) {
	var in struct {
		HostID string `json:"hostId"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var tournamentName, status, mapName, teamBSide string
	var mapSequence []string
	var playersPerSide, bestOf int
	err = tx.QueryRow(ctx, `SELECT t.name,t.status,t.players_per_side,t.best_of,m.selected_map,m.team_b_side,m.map_sequence FROM tournaments t JOIN tournament_matches m ON m.tournament_id=t.id WHERE t.id=$1 AND m.id=$2 AND m.status='pending' AND m.team_a IS NOT NULL AND m.team_b IS NOT NULL AND m.room_id IS NULL AND m.veto_step=7 AND m.selected_map IS NOT NULL AND m.team_b_side IS NOT NULL FOR UPDATE OF t,m`, r.PathValue("id"), r.PathValue("match")).Scan(&tournamentName, &status, &playersPerSide, &bestOf, &mapName, &teamBSide, &mapSequence)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusConflict, "This fixture is unavailable or already has a match room")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != "active" {
		fail(w, http.StatusConflict, "Start the tournament bracket before opening its fixtures")
		return
	}
	var earlierRoundPending bool
	if err = tx.QueryRow(ctx, `SELECT t.format='round_robin' AND EXISTS(SELECT 1 FROM tournament_matches previous WHERE previous.tournament_id=t.id AND previous.round<m.round AND previous.status='pending') FROM tournaments t JOIN tournament_matches m ON m.tournament_id=t.id WHERE t.id=$1 AND m.id=$2`, r.PathValue("id"), r.PathValue("match")).Scan(&earlierRoundPending); err != nil {
		internal(w, err)
		return
	}
	if earlierRoundPending {
		fail(w, http.StatusConflict, "Complete the previous round before opening this fixture")
		return
	}
	if len(mapSequence) == 0 {
		mapSequence = []string{mapName}
	}
	if len(mapSequence) != 1 && len(mapSequence) != 3 && len(mapSequence) != 5 {
		fail(w, http.StatusConflict, "The fixture map sequence is invalid")
		return
	}
	mapSides := make([]string, len(mapSequence))
	for i := range mapSides {
		mapSides[i] = "team1_ct"
		if i%2 == 1 {
			mapSides[i] = "team1_t"
		}
	}
	host, err := lockHost(ctx, tx, in.HostID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Server not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !canOpenMatch(host) {
		fail(w, http.StatusConflict, "Choose a running Match server with a verified SteamID roster")
		return
	}
	if bestOf > 1 && !host.SupportsMatchSeries {
		fail(w, http.StatusConflict, "Update the server agent and Clutchbase Access reporter before opening a BO3 or BO5 room")
		return
	}
	var teamA, teamB string
	if err = tx.QueryRow(ctx, "SELECT team_a,team_b FROM tournament_matches WHERE id=$1", r.PathValue("match")).Scan(&teamA, &teamB); err != nil {
		internal(w, err)
		return
	}
	teamRows, err := tx.Query(ctx, "SELECT id FROM teams WHERE id=ANY($1::text[]) ORDER BY id FOR UPDATE", []string{teamA, teamB})
	if err != nil {
		internal(w, err)
		return
	}
	lockedTeams := 0
	for teamRows.Next() {
		var ignored string
		if err = teamRows.Scan(&ignored); err != nil {
			teamRows.Close()
			internal(w, err)
			return
		}
		lockedTeams++
	}
	err = teamRows.Err()
	teamRows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if lockedTeams != 2 {
		fail(w, http.StatusConflict, "One of the fixture teams no longer exists")
		return
	}
	rosters := map[string][]string{}
	for _, teamID := range []string{teamA, teamB} {
		rows, queryErr := tx.Query(ctx, `SELECT steam_id FROM team_members WHERE team_id=$1 ORDER BY joined_at,steam_id LIMIT $2`, teamID, playersPerSide)
		if queryErr != nil {
			internal(w, queryErr)
			return
		}
		for rows.Next() {
			var steamID string
			if queryErr = rows.Scan(&steamID); queryErr != nil {
				rows.Close()
				internal(w, queryErr)
				return
			}
			rosters[teamID] = append(rosters[teamID], steamID)
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			internal(w, queryErr)
			return
		}
		if len(rosters[teamID]) != playersPerSide {
			fail(w, http.StatusConflict, "Both teams need a full tournament roster before a match room can open")
			return
		}
	}
	allowed := make(map[string]bool, len(host.Whitelist))
	for _, steamID := range host.Whitelist {
		allowed[steamID] = true
	}
	for _, teamID := range []string{teamA, teamB} {
		for _, steamID := range rosters[teamID] {
			if !allowed[steamID] {
				fail(w, http.StatusConflict, "Add every fixture player to the selected server roster before opening the match")
				return
			}
		}
	}
	var round, number int
	if err = tx.QueryRow(ctx, "SELECT round,number FROM tournament_matches WHERE id=$1", r.PathValue("match")).Scan(&round, &number); err != nil {
		internal(w, err)
		return
	}
	title := []rune(fmt.Sprintf("%s · Round %d · Match %d", tournamentName, round, number))
	if len(title) > 80 {
		title = title[:80]
	}
	var occupied bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND status='open')", host.ID).Scan(&occupied); err != nil {
		internal(w, err)
		return
	}
	if occupied {
		fail(w, http.StatusConflict, "This server already has an open match room")
		return
	}
	roomID := rand.Text()
	if _, err = tx.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,players_per_side,series_maps,series_sides) VALUES($1,$2,$3,$4,$5,$6,$7)`, roomID, host.ID, string(title), mapName, playersPerSide, mapSequence, mapSides); err != nil {
		internal(w, err)
		return
	}
	teamASide := "ct"
	if teamBSide == "ct" {
		teamASide = "t"
	}
	for _, side := range []struct {
		team string
		name string
	}{{teamA, teamASide}, {teamB, teamBSide}} {
		for _, steamID := range rosters[side.team] {
			if _, err = tx.Exec(ctx, `INSERT INTO match_room_players(match_id,steam_id,side) VALUES($1,$2,$3)`, roomID, steamID, side.name); err != nil {
				internal(w, err)
				return
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET room_id=$2 WHERE id=$1 AND room_id IS NULL`, r.PathValue("match"), roomID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusCreated, map[string]string{"roomId": roomID})
}
