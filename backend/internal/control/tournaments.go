package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/bits"
	"net/http"
	"strings"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

func (a *API) registerTournamentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/player/teams", a.createTeam)
	mux.HandleFunc("POST /api/player/teams/join", a.joinTeam)
	mux.HandleFunc("POST /api/player/teams/{id}/invite", a.inviteTeam)
	mux.HandleFunc("POST /api/player/teams/leave", a.leaveTeam)
	mux.HandleFunc("PATCH /api/player/teams/{id}/captain", a.manageTeam)
	mux.HandleFunc("DELETE /api/player/teams/{id}/members/{steam}", a.manageTeam)
	mux.HandleFunc("DELETE /api/player/teams/{id}", a.manageTeam)
	mux.HandleFunc("POST /api/tournaments", a.admin(a.createTournament))
	mux.HandleFunc("POST /api/tournaments/{id}/checkin", a.admin(a.startTournamentCheckin))
	mux.HandleFunc("POST /api/tournaments/{id}/cancel", a.admin(a.cancelTournament))
	mux.HandleFunc("POST /api/tournaments/{id}/bracket", a.admin(a.buildTournamentBracket))
	mux.HandleFunc("PATCH /api/tournaments/{id}/matches/{match}/winner", a.admin(a.reportTournamentWinner))
	mux.HandleFunc("PATCH /api/tournaments/{id}/matches/{match}/correction", a.admin(a.correctTournamentWinner))
	mux.HandleFunc("PATCH /api/tournaments/{id}/matches/{match}/dispute", a.admin(a.resolveTournamentDispute))
	mux.HandleFunc("POST /api/player/tournaments/{id}/matches/{match}/dispute", a.submitTournamentDispute)
	mux.HandleFunc("PATCH /api/tournaments/{id}/matches/{match}/schedule", a.admin(a.scheduleTournamentMatch))
	mux.HandleFunc("POST /api/tournaments/{id}/matches/{match}/room", a.admin(a.openTournamentMatchRoom))
	mux.HandleFunc("POST /api/player/tournaments/{id}/register", a.registerTournamentTeam)
	mux.HandleFunc("POST /api/player/tournaments/{id}/checkin", a.checkinTournamentTeam)
	mux.HandleFunc("DELETE /api/player/tournaments/{id}/registration", a.withdrawTournamentTeam)
	mux.HandleFunc("POST /api/player/tournaments/{id}/matches/{match}/veto", a.submitTournamentVeto)
}

func (a *API) teams(ctx context.Context) ([]protocol.Team, error) {
	rows, err := a.DB.Query(ctx, `SELECT t.id,t.name,t.captain_steam_id,COALESCE(json_agg(json_build_object('steamId',m.steam_id,'gameName',p.game_name) ORDER BY m.joined_at,m.steam_id) FILTER(WHERE m.steam_id IS NOT NULL),'[]'::json),t.revision,(`+teamRosterLockedSQL+`),(`+teamRegisteredSQL+`)
		FROM teams t LEFT JOIN team_members m ON m.team_id=t.id LEFT JOIN players p ON p.steam_id=m.steam_id WHERE t.disbanded_at IS NULL GROUP BY t.id ORDER BY lower(t.name),t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []protocol.Team{}
	for rows.Next() {
		var team protocol.Team
		var members []byte
		if err := rows.Scan(&team.ID, &team.Name, &team.CaptainSteamID, &members, &team.Revision, &team.RosterLocked, &team.Registered); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(members, &team.Members); err != nil {
			return nil, err
		}
		out = append(out, team)
	}
	return out, rows.Err()
}

func (a *API) tournaments(ctx context.Context) ([]protocol.Tournament, error) {
	rows, err := a.DB.Query(ctx, `SELECT id,name,players_per_side,status,COALESCE(winner_team_id,''),map_pool,best_of,format FROM tournaments ORDER BY created_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []protocol.Tournament{}
	for rows.Next() {
		var t protocol.Tournament
		if err := rows.Scan(&t.ID, &t.Name, &t.PlayersPerSide, &t.Status, &t.WinnerTeamID, &t.MapPool, &t.BestOf, &t.Format); err != nil {
			return nil, err
		}
		t.Entries = []protocol.TournamentEntry{}
		t.Matches = []protocol.TournamentMatch{}
		checkedIn := 0
		entries, err := a.DB.Query(ctx, `SELECT e.team_id,t.name,e.status,(SELECT count(*) FROM team_members WHERE team_id=e.team_id)::int FROM tournament_entries e JOIN teams t ON t.id=e.team_id WHERE e.tournament_id=$1 ORDER BY lower(t.name),e.team_id`, t.ID)
		if err != nil {
			return nil, err
		}
		for entries.Next() {
			var e protocol.TournamentEntry
			if err := entries.Scan(&e.TeamID, &e.TeamName, &e.Status, &e.Members); err != nil {
				entries.Close()
				return nil, err
			}
			t.Entries = append(t.Entries, e)
			if e.Status == "checked_in" {
				checkedIn++
			}
		}
		if err := entries.Err(); err != nil {
			entries.Close()
			return nil, err
		}
		entries.Close()
		games, err := a.DB.Query(ctx, `SELECT m.id,m.bracket_stage,m.round,m.number,m.status,m.team_a,COALESCE(a.name,''),m.team_b,COALESCE(b.name,''),COALESCE(m.winner_team_id,''),COALESCE(m.room_id,''),COALESCE(r.status,''),COALESCE(r.map,m.selected_map,''),COALESCE(r.status='closed' AND r.load_status='complete' AND ((t.best_of>1 AND (r.reported_team1_score IS NULL OR r.reported_team2_score IS NULL OR NOT ((r.reported_team1_score=t.best_of/2+1 AND r.reported_team2_score<t.best_of/2+1) OR (r.reported_team2_score=t.best_of/2+1 AND r.reported_team1_score<t.best_of/2+1)))) OR (t.best_of=1 AND ((r.reported_ct IS NOT NULL AND r.reported_t IS NOT NULL AND r.reported_ct=r.reported_t) OR (r.reported_ct IS NULL AND r.result_ct IS NOT NULL)))),false),m.veto_bans,m.veto_maps,m.map_sequence,m.veto_step,COALESCE(m.veto_turn_team_id,''),COALESCE(turn.name,''),COALESCE(m.team_b_side,''),r.reported_ct,r.reported_t,r.result_ct,r.result_t,r.reported_team1_score,r.reported_team2_score,m.scheduled_at,m.schedule_revision,COALESCE(d.reason,''),COALESCE(d.resolution,''),COALESCE(d.revision,0),COALESCE(ARRAY(SELECT steam_id FROM match_room_players WHERE match_id=m.room_id),ARRAY[]::text[]),COALESCE(r.status='closed' AND r.load_status='complete' AND ((r.reported_ct IS NOT NULL AND r.reported_t IS NOT NULL) OR (r.reported_team1_score IS NOT NULL AND r.reported_team2_score IS NOT NULL)),false),m.result_revision,EXISTS(SELECT 1 FROM tournament_winner_history wh WHERE wh.match_id=m.id AND wh.previous_winner IS NOT NULL),COALESCE((SELECT json_agg(json_build_object('previousWinner',COALESCE(old.name,wh.previous_winner,''),'newWinner',COALESCE(new.name,wh.new_winner,''),'revision',wh.revision,'changedAt',wh.changed_at) ORDER BY wh.revision) FROM tournament_winner_history wh LEFT JOIN teams old ON old.id=wh.previous_winner LEFT JOIN teams new ON new.id=wh.new_winner WHERE wh.match_id=m.id AND wh.previous_winner IS NOT NULL),'[]'::json) FROM tournament_matches m JOIN tournaments t ON t.id=m.tournament_id LEFT JOIN teams a ON a.id=m.team_a LEFT JOIN teams b ON b.id=m.team_b LEFT JOIN teams turn ON turn.id=m.veto_turn_team_id LEFT JOIN match_rooms r ON r.id=m.room_id LEFT JOIN tournament_disputes d ON d.match_id=m.id WHERE m.tournament_id=$1 ORDER BY m.round,m.number`, t.ID)
		if err != nil {
			return nil, err
		}
		for games.Next() {
			var m protocol.TournamentMatch
			var aid, bid *string
			var team1Score, team2Score *int
			var an, bn string
			var canReport bool
			var winnerHistory []byte
			var reason, resolution string
			var disputeRevision int64
			if err := games.Scan(&m.ID, &m.BracketStage, &m.Round, &m.Number, &m.Status, &aid, &an, &bid, &bn, &m.WinnerTeamID, &m.RoomID, &m.RoomStatus, &m.Map, &canReport, &m.VetoBans, &m.VetoMaps, &m.MapSequence, &m.VetoStep, &m.VetoTurnTeamID, &m.VetoTurnTeam, &m.TeamBSide, &m.ReportedCT, &m.ReportedT, &m.ResultCT, &m.ResultT, &team1Score, &team2Score, &m.ScheduledAt, &m.ScheduleRevision, &reason, &resolution, &disputeRevision, &m.DisputeParticipants, &m.DisputeEligible, &m.ResultRevision, &m.WinnerCorrected, &winnerHistory); err != nil {
				games.Close()
				return nil, err
			}
			if err := json.Unmarshal(winnerHistory, &m.WinnerHistory); err != nil {
				games.Close()
				return nil, err
			}
			if reason != "" {
				m.Dispute = &protocol.TournamentDispute{Reason: reason, Resolution: resolution, Revision: disputeRevision}
			}
			if m.VetoBans == nil {
				m.VetoBans = []string{}
			}
			if m.VetoMaps == nil {
				m.VetoMaps = []string{}
			}
			if m.MapSequence == nil {
				m.MapSequence = []string{}
			}
			if aid != nil {
				m.TeamA = &protocol.TournamentEntry{TeamID: *aid, TeamName: an}
			}
			if bid != nil {
				m.TeamB = &protocol.TournamentEntry{TeamID: *bid, TeamName: bn}
			}
			if team1Score != nil && team2Score != nil {
				m.TeamAScore, m.TeamBScore = team1Score, team2Score
				if m.TeamBSide == "ct" {
					m.TeamAScore, m.TeamBScore = team2Score, team1Score
				}
			}
			m.CanReportWinner = t.Status == "active" && m.Status == "pending" && m.TeamA != nil && m.TeamB != nil && canReport
			m.CanCorrectWinner = t.Format == "round_robin" && (t.Status == "active" || t.Status == "complete") && m.Status == "complete" && m.TeamA != nil && m.TeamB != nil && m.WinnerTeamID != ""
			m.CanOpenRoom = t.Status == "active" && m.Status == "pending" && m.TeamA != nil && m.TeamB != nil && m.RoomID == "" && m.VetoStep == 7 && m.Map != ""
			m.CanSchedule = t.Status == "active" && m.Status == "pending"
			t.Matches = append(t.Matches, m)
		}
		if err := games.Err(); err != nil {
			games.Close()
			return nil, err
		}
		games.Close()
		if t.Status == "active" && t.Format == "double_elimination" {
			rows, err := a.DB.Query(ctx, `SELECT m.id FROM tournament_matches m JOIN tournament_matches wn ON wn.id=m.winner_next_match_id AND wn.tournament_id=m.tournament_id LEFT JOIN tournament_matches ln ON ln.id=m.loser_next_match_id AND ln.tournament_id=m.tournament_id WHERE m.tournament_id=$1 AND m.bracket_stage IN ('winners','losers') AND m.status='complete' AND m.team_a IS NOT NULL AND m.team_b IS NOT NULL AND m.winner_team_id IS NOT NULL AND wn.status='pending' AND wn.veto_step=0 AND wn.room_id IS NULL AND (CASE m.winner_next_slot WHEN 1 THEN wn.team_a WHEN 2 THEN wn.team_b END)=m.winner_team_id AND ((m.bracket_stage='winners' AND ln.status='pending' AND ln.veto_step=0 AND ln.room_id IS NULL AND (CASE m.loser_next_slot WHEN 1 THEN ln.team_a WHEN 2 THEN ln.team_b END)=CASE WHEN m.winner_team_id=m.team_a THEN m.team_b ELSE m.team_a END) OR (m.bracket_stage='losers' AND EXISTS(SELECT 1 FROM tournament_entries e WHERE e.tournament_id=m.tournament_id AND e.team_id=m.winner_team_id AND e.status='checked_in') AND EXISTS(SELECT 1 FROM tournament_entries e WHERE e.tournament_id=m.tournament_id AND e.team_id=CASE WHEN m.winner_team_id=m.team_a THEN m.team_b ELSE m.team_a END AND e.status='eliminated'))) `, t.ID)
			if err != nil {
				return nil, err
			}
			eligible := map[string]bool{}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					rows.Close()
					return nil, err
				}
				eligible[id] = true
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return nil, err
			}
			rows.Close()
			for i := range t.Matches {
				t.Matches[i].CanCorrectWinner = eligible[t.Matches[i].ID]
			}
			var grandID string
			err = a.DB.QueryRow(ctx, `SELECT m.id FROM tournament_matches m JOIN tournament_matches n ON n.id=m.winner_next_match_id AND n.tournament_id=m.tournament_id WHERE m.tournament_id=$1 AND m.bracket_stage='grand_final' AND m.status='complete' AND m.team_a IS NOT NULL AND m.winner_team_id=m.team_b AND n.bracket_stage='reset_final' AND n.status='pending' AND n.team_a=m.team_b AND n.team_b=m.team_a AND n.winner_team_id IS NULL AND n.room_id IS NULL AND n.veto_step=0`, t.ID).Scan(&grandID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			for i := range t.Matches {
				if t.Matches[i].ID == grandID {
					t.Matches[i].CanCorrectWinner = true
				}
			}
		}
		if t.Status == "active" && t.Format == "single_elimination" {
			for i := range t.Matches {
				m := &t.Matches[i]
				if m.Status != "complete" || m.TeamA == nil || m.TeamB == nil || m.WinnerTeamID == "" {
					continue
				}
				for _, next := range t.Matches {
					if next.Round != m.Round+1 || next.Number != (m.Number+1)/2 || next.Status != "pending" || next.RoomID != "" || next.VetoStep != 0 {
						continue
					}
					slot := next.TeamA
					if m.Number%2 == 0 {
						slot = next.TeamB
					}
					m.CanCorrectWinner = slot != nil && slot.TeamID == m.WinnerTeamID
				}
			}
		}
		if t.Status == "complete" && (t.Format == "single_elimination" || t.Format == "double_elimination") {
			maxRound := 0
			for _, m := range t.Matches {
				if m.Round > maxRound {
					maxRound = m.Round
				}
			}
			for i := range t.Matches {
				m := &t.Matches[i]
				terminal := (t.Format == "single_elimination" && m.Round == maxRound) || (t.Format == "double_elimination" && m.BracketStage == "reset_final")
				m.CanCorrectWinner = terminal && m.Status == "complete" && m.TeamA != nil && m.TeamB != nil && m.WinnerTeamID != ""
			}
		}
		if t.Status == "complete" && t.Format == "double_elimination" {
			var grandID string
			err := a.DB.QueryRow(ctx, `SELECT m.id FROM tournament_matches m JOIN tournament_matches n ON n.id=m.winner_next_match_id AND n.tournament_id=m.tournament_id WHERE m.tournament_id=$1 AND m.bracket_stage='grand_final' AND m.status='complete' AND m.winner_team_id=m.team_a AND m.team_b IS NOT NULL AND n.bracket_stage='reset_final' AND n.status='skipped' AND n.team_a IS NULL AND n.team_b IS NULL AND n.winner_team_id IS NULL AND n.room_id IS NULL AND n.veto_step=0`, t.ID).Scan(&grandID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			for i := range t.Matches {
				if t.Matches[i].ID == grandID {
					t.Matches[i].CanCorrectWinner = true
				}
			}
		}
		if t.Format == "round_robin" {
			earliestPending := 0
			for _, match := range t.Matches {
				if match.Status == "pending" && (earliestPending == 0 || match.Round < earliestPending) {
					earliestPending = match.Round
				}
			}
			for i := range t.Matches {
				if earliestPending > 0 && t.Matches[i].Round > earliestPending {
					t.Matches[i].CanOpenRoom = false
				}
			}
		}
		t.Standings = tournamentStandings(t)
		t.CanCancel = (t.Status == "open" || t.Status == "checkin") && len(t.Matches) == 0
		t.CanCheckIn = t.Status == "open" && len(t.Entries) >= 2
		t.CanBuildBracket = t.Status == "checkin" && checkedIn >= 2
		out = append(out, t)
	}
	return out, rows.Err()
}

func (a *API) createTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 48 {
		fail(w, 400, "Team name must be 1 to 48 characters")
		return
	}
	id := rand.Text()
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var existing bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM team_members WHERE steam_id=$1)`, p.SteamID).Scan(&existing); err != nil {
		internal(w, err)
		return
	}
	if existing {
		fail(w, 409, "Leave your current team before creating another")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$2,$3)`, id, in.Name, p.SteamID); err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO team_members(team_id,steam_id) VALUES($1,$2)`, id, p.SteamID)
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]string{"id": id, "name": in.Name})
}

func (a *API) joinTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Code = strings.TrimSpace(in.Code)
	if len(in.Code) < 20 || len(in.Code) > 128 {
		fail(w, 400, "Enter a valid team invite")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var team string
	err = tx.QueryRow(r.Context(), `SELECT i.team_id FROM team_invites i JOIN teams t ON t.id=i.team_id WHERE i.code_hash=$1 AND i.expires_at>now() AND t.disbanded_at IS NULL`, digest(in.Code)).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Invite is invalid or expired")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var activeTeam string
	err = tx.QueryRow(r.Context(), `SELECT id FROM teams WHERE id=$1 AND disbanded_at IS NULL FOR UPDATE`, team).Scan(&activeTeam)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Invite is invalid or expired")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var validInvite bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM team_invites WHERE team_id=$1 AND code_hash=$2 AND expires_at>now())`, team, digest(in.Code)).Scan(&validInvite); err != nil {
		internal(w, err)
		return
	}
	if !validInvite {
		fail(w, 404, "Invite is invalid or expired")
		return
	}
	var locked bool
	if err = tx.QueryRow(r.Context(), "SELECT "+teamRosterLockedSQL+" FROM teams t WHERE t.id=$1", team).Scan(&locked); err != nil {
		internal(w, err)
		return
	}
	if locked {
		fail(w, 409, "The team roster is locked while checked in to a tournament")
		return
	}
	var current string
	if err = tx.QueryRow(r.Context(), `SELECT team_id FROM team_members WHERE steam_id=$1`, p.SteamID).Scan(&current); err == nil {
		fail(w, 409, "Leave your current team before joining another")
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internal(w, err)
		return
	}
	var size int
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM team_members WHERE team_id=$1`, team).Scan(&size); err != nil {
		internal(w, err)
		return
	}
	if size >= 5 {
		fail(w, 409, "This team already has five players")
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO team_members(team_id,steam_id) VALUES($1,$2)`, team, p.SteamID); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE teams SET revision=revision+1 WHERE id=$1", team); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 204, nil)
}

func (a *API) leaveTeam(w http.ResponseWriter, r *http.Request) {
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
	var team string
	err = tx.QueryRow(r.Context(), `SELECT t.id FROM teams t JOIN team_members m ON m.team_id=t.id WHERE m.steam_id=$1 AND t.captain_steam_id<>$1 AND t.disbanded_at IS NULL FOR UPDATE OF t`, p.SteamID).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Captains must transfer captaincy or disband before leaving")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var locked bool
	if err = tx.QueryRow(r.Context(), "SELECT "+teamRosterLockedSQL+" FROM teams t WHERE t.id=$1", team).Scan(&locked); err != nil {
		internal(w, err)
		return
	}
	if locked {
		fail(w, 409, "The team roster is locked while checked in to a tournament")
		return
	}
	tag, err := tx.Exec(r.Context(), `DELETE FROM team_members WHERE team_id=$1 AND steam_id=$2`, team, p.SteamID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "You are not on a team")
		return
	}
	if _, err = tx.Exec(r.Context(), "UPDATE teams SET revision=revision+1 WHERE id=$1", team); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 204, nil)
}

func (a *API) createTournament(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name           string   `json:"name"`
		Format         string   `json:"format"`
		PlayersPerSide int      `json:"playersPerSide"`
		BestOf         int      `json:"bestOf"`
		MapPool        []string `json:"mapPool"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Format == "" {
		in.Format = "single_elimination"
	}
	if in.Format != "single_elimination" && in.Format != "round_robin" && in.Format != "double_elimination" {
		fail(w, 400, "Choose single elimination, double elimination or round robin")
		return
	}
	if in.Name == "" || len(in.Name) > 80 || in.PlayersPerSide < 1 || in.PlayersPerSide > 5 {
		fail(w, 400, "Provide a name and team size from 1v1 to 5v5")
		return
	}
	if in.BestOf == 0 {
		in.BestOf = 1
	}
	if in.BestOf != 1 && in.BestOf != 3 && in.BestOf != 5 {
		fail(w, 400, "Choose best of 1, 3 or 5")
		return
	}
	if len(in.MapPool) == 0 {
		in.MapPool = append([]string(nil), protocol.Maps[:7]...)
	}
	if len(in.MapPool) != 7 {
		fail(w, 400, "Choose exactly seven official maps for the tournament veto")
		return
	}
	seenMaps := make(map[string]bool, len(in.MapPool))
	for _, mapName := range in.MapPool {
		if !protocol.ValidMap(mapName) || seenMaps[mapName] {
			fail(w, 400, "Map pool must contain seven different supported maps")
			return
		}
		seenMaps[mapName] = true
	}
	id := rand.Text()
	if _, err := a.DB.Exec(r.Context(), `INSERT INTO tournaments(id,name,players_per_side,status,map_pool,best_of,format) VALUES($1,$2,$3,'open',$4,$5,$6)`, id, in.Name, in.PlayersPerSide, in.MapPool, in.BestOf, in.Format); err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]string{"id": id})
}

func (a *API) startTournamentCheckin(w http.ResponseWriter, r *http.Request) {
	tag, err := a.DB.Exec(r.Context(), `UPDATE tournaments SET status='checkin' WHERE id=$1 AND status='open'`, r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Tournament is not open for check-in")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) registerTournamentTeam(w http.ResponseWriter, r *http.Request) {
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
	var needed int
	var status string
	var entries int
	err = tx.QueryRow(r.Context(), `SELECT players_per_side,status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&needed, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var team string
	var size int
	err = tx.QueryRow(r.Context(), `SELECT t.id FROM teams t WHERE t.captain_steam_id=$1 AND t.disbanded_at IS NULL FOR UPDATE`, p.SteamID).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Only a team captain can register")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM team_members WHERE team_id=$1`, team).Scan(&size); err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM tournament_entries WHERE tournament_id=$1`, r.PathValue("id")).Scan(&entries); err != nil {
		internal(w, err)
		return
	}
	if status != "open" || size < needed || entries >= 64 {
		fail(w, 409, "Tournament registration is closed or your team is too small")
		return
	}
	tag, err := tx.Exec(r.Context(), `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'registered') ON CONFLICT DO NOTHING`, r.PathValue("id"), team)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Your team is already registered")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) checkinTournamentTeam(w http.ResponseWriter, r *http.Request) {
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
	var needed int
	var status string
	err = tx.QueryRow(r.Context(), `SELECT players_per_side,status FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&needed, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var team string
	var size int
	err = tx.QueryRow(r.Context(), `SELECT t.id FROM teams t WHERE t.captain_steam_id=$1 AND t.disbanded_at IS NULL FOR UPDATE`, p.SteamID).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Only a team captain can check in")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM team_members WHERE team_id=$1`, team).Scan(&size); err != nil {
		internal(w, err)
		return
	}
	if status != "checkin" || size < needed {
		fail(w, 409, "Your team cannot check in now; verify your roster and tournament status")
		return
	}
	tag, err := tx.Exec(r.Context(), `UPDATE tournament_entries SET status='checked_in' WHERE tournament_id=$1 AND team_id=$2 AND status='registered'`, r.PathValue("id"), team)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Your team cannot check in now")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) buildTournamentBracket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var status, format string
	if err = tx.QueryRow(ctx, `SELECT status,format FROM tournaments WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&status, &format); errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if status != "checkin" {
		fail(w, 409, "Start check-in before creating the bracket")
		return
	}
	rows, err := tx.Query(ctx, `SELECT team_id FROM tournament_entries WHERE tournament_id=$1 AND status='checked_in' ORDER BY team_id`, r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		internal(w, err)
		return
	}
	rows.Close()
	if len(ids) < 2 {
		fail(w, 409, "At least two teams must check in")
		return
	}
	if format == "round_robin" {
		for _, fixture := range roundRobinFixtures(ids) {
			if _, err = tx.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,veto_turn_team_id) VALUES($1,$2,$3,$4,$5,$6,'pending',$5)`, rand.Text(), r.PathValue("id"), fixture.round, fixture.number, fixture.a, fixture.b); err != nil {
				internal(w, err)
				return
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='active' WHERE id=$1`, r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
		if err = tx.Commit(ctx); err != nil {
			internal(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if format == "double_elimination" {
		for _, m := range doubleEliminationBracket(ids) {
			var a, b, winnerNext, winnerSlot, loserNext, loserSlot, sourceA, sourceB any
			if m.a != "" {
				a = m.a
			}
			if m.b != "" {
				b = m.b
			}
			if m.winnerMatch != "" {
				winnerNext = m.winnerMatch
				winnerSlot = m.winnerSlot
			}
			if m.loserMatch != "" {
				loserNext = m.loserMatch
				loserSlot = m.loserSlot
			}
			if m.sourceA != "" {
				sourceA = m.sourceA
			}
			if m.sourceB != "" {
				sourceB = m.sourceB
			}
			if _, err = tx.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,bracket_stage,winner_next_match_id,winner_next_slot,loser_next_match_id,loser_next_slot,source_a_match_id,source_b_match_id) VALUES($1,$2,$3,$4,$5,$6,'pending',$7,$8,$9,$10,$11,$12,$13)`, m.id, r.PathValue("id"), m.round, m.number, a, b, m.stage, winnerNext, winnerSlot, loserNext, loserSlot, sourceA, sourceB); err != nil {
				internal(w, err)
				return
			}
		}
		if err = settleDoubleEliminationByes(ctx, tx, r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='active' WHERE id=$1`, r.PathValue("id")); err != nil {
			internal(w, err)
			return
		}
		if err = tx.Commit(ctx); err != nil {
			internal(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	size := 1
	for size < len(ids) {
		size *= 2
	}
	for i := 0; i < size/2; i++ {
		var aID, bID any
		state := "pending"
		winner := ""
		if i*2 < len(ids) {
			aID = ids[i*2]
		}
		if i*2+1 < len(ids) {
			bID = ids[i*2+1]
		} else if aID != nil {
			state = "bye"
			winner = ids[i*2]
		}
		id := rand.Text()
		var firstVetoTeam any
		if state == "pending" && aID != nil && bID != nil {
			firstVetoTeam = aID
		}
		if _, err = tx.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,winner_team_id,status,veto_turn_team_id) VALUES($1,$2,1,$3,$4,$5,NULLIF($6,''),$7,$8)`, id, r.PathValue("id"), i+1, aID, bID, winner, state, firstVetoTeam); err != nil {
			internal(w, err)
			return
		}
	}
	for round, count := 2, size/4; count >= 1; round, count = round+1, count/2 {
		for n := 1; n <= count; n++ {
			if _, err = tx.Exec(ctx, `INSERT INTO tournament_matches(id,tournament_id,round,number,status) VALUES($1,$2,$3,$4,'pending')`, rand.Text(), r.PathValue("id"), round, n); err != nil {
				internal(w, err)
				return
			}
		}
	}
	for round := 1; round <= bits.Len(uint(size))-1; round++ {
		rows, err = tx.Query(ctx, `SELECT m.number,m.team_a,m.team_b FROM tournament_matches m WHERE m.tournament_id=$1 AND m.round=$2 AND m.status='pending' AND (m.round=1 OR NOT EXISTS (SELECT 1 FROM tournament_matches prev WHERE prev.tournament_id=m.tournament_id AND prev.round=m.round-1 AND prev.number IN (m.number*2-1,m.number*2) AND prev.status='pending')) ORDER BY m.number`, r.PathValue("id"), round)
		if err != nil {
			internal(w, err)
			return
		}
		type slot struct {
			number int
			a, b   *string
		}
		slots := []slot{}
		for rows.Next() {
			var m slot
			if err = rows.Scan(&m.number, &m.a, &m.b); err != nil {
				rows.Close()
				internal(w, err)
				return
			}
			if m.a == nil || m.b == nil {
				slots = append(slots, m)
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		rows.Close()
		for _, m := range slots {
			if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET status='bye',winner_team_id=COALESCE(team_a,team_b) WHERE tournament_id=$1 AND round=$2 AND number=$3`, r.PathValue("id"), round, m.number); err != nil {
				internal(w, err)
				return
			}
		}
		if round < bits.Len(uint(size))-1 {
			if _, err = tx.Exec(ctx, `UPDATE tournament_matches next SET team_a=prev.winner_team_id FROM tournament_matches prev WHERE prev.tournament_id=$1 AND prev.round=$2 AND prev.status='bye' AND prev.winner_team_id IS NOT NULL AND prev.number%2=1 AND next.tournament_id=prev.tournament_id AND next.round=$2+1 AND next.number=(prev.number+1)/2`, r.PathValue("id"), round); err == nil {
				_, err = tx.Exec(ctx, `UPDATE tournament_matches next SET team_b=prev.winner_team_id FROM tournament_matches prev WHERE prev.tournament_id=$1 AND prev.round=$2 AND prev.status='bye' AND prev.winner_team_id IS NOT NULL AND prev.number%2=0 AND next.tournament_id=prev.tournament_id AND next.round=$2+1 AND next.number=prev.number/2`, r.PathValue("id"), round)
			}
			if err != nil {
				internal(w, err)
				return
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET veto_turn_team_id=team_a WHERE tournament_id=$1 AND status='pending' AND team_a IS NOT NULL AND team_b IS NOT NULL`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE tournaments SET status='active' WHERE id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) reportTournamentWinner(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TeamID string `json:"teamId"`
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
	var round, number int
	var teamA, teamB *string
	var resultReady bool
	err = tx.QueryRow(ctx, `SELECT m.round,m.number,m.team_a,m.team_b,COALESCE(r.status='closed' AND r.load_status='complete' AND ((t.best_of>1 AND (r.reported_team1_score IS NULL OR r.reported_team2_score IS NULL OR NOT ((r.reported_team1_score=t.best_of/2+1 AND r.reported_team2_score<t.best_of/2+1) OR (r.reported_team2_score=t.best_of/2+1 AND r.reported_team1_score<t.best_of/2+1)))) OR (t.best_of=1 AND ((r.reported_ct IS NOT NULL AND r.reported_t IS NOT NULL AND r.reported_ct=r.reported_t) OR (r.reported_ct IS NULL AND r.result_ct IS NOT NULL)))),false) FROM tournament_matches m JOIN tournaments t ON t.id=m.tournament_id LEFT JOIN match_rooms r ON r.id=m.room_id WHERE m.id=$1 AND m.tournament_id=$2 AND m.status='pending' AND t.status='active' FOR UPDATE OF m,t`, r.PathValue("match"), r.PathValue("id")).Scan(&round, &number, &teamA, &teamB, &resultReady)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Pending tournament match not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !resultReady {
		fail(w, http.StatusConflict, "Close the loaded tournament match and record its result before advancing the bracket")
		return
	}
	if teamA == nil || teamB == nil || (in.TeamID != *teamA && in.TeamID != *teamB) {
		fail(w, 400, "Winner must be one of the teams in this match")
		return
	}
	if err = advanceTournamentWinner(ctx, tx, r.PathValue("id"), r.PathValue("match"), round, number, in.TeamID); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
