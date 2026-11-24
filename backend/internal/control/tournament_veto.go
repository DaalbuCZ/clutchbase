package control

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

func tournamentVetoPick(bestOf, step int) bool {
	return (bestOf == 3 && step >= 2 && step <= 3) || (bestOf == 5 && step >= 2 && step <= 5)
}

func tournamentVetoTurn(bestOf, nextStep int, teamA, teamB string) string {
	if bestOf == 1 {
		switch nextStep {
		case 1:
			return teamA
		case 2, 3, 4:
			return teamB
		case 5:
			return teamA
		default:
			return teamB
		}
	}
	if nextStep == 6 {
		return teamB
	}
	if nextStep%2 == 0 {
		return teamA
	}
	return teamB
}

func (a *API) submitTournamentVeto(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Map  string `json:"map"`
		Side string `json:"side"`
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
	var tournamentStatus, matchStatus, turnTeam, teamA, teamB string
	var step, bestOf int
	var bans, picks, sequence, mapPool []string
	err = tx.QueryRow(ctx, `SELECT t.status,m.status,COALESCE(m.veto_turn_team_id,''),COALESCE(m.team_a,''),COALESCE(m.team_b,''),m.veto_step,m.veto_bans,m.veto_maps,m.map_sequence,t.map_pool,t.best_of
		FROM tournaments t JOIN tournament_matches m ON m.tournament_id=t.id WHERE t.id=$1 AND m.id=$2 FOR UPDATE OF t,m`, r.PathValue("id"), r.PathValue("match")).Scan(&tournamentStatus, &matchStatus, &turnTeam, &teamA, &teamB, &step, &bans, &picks, &sequence, &mapPool, &bestOf)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "Tournament fixture not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if tournamentStatus != "active" || matchStatus != "pending" || teamA == "" || teamB == "" || step > 6 {
		fail(w, http.StatusConflict, "This fixture is not accepting veto actions")
		return
	}
	var playerTeam string
	err = tx.QueryRow(ctx, `SELECT team_id FROM team_members WHERE steam_id=$1 AND team_id=ANY($2::text[])`, p.SteamID, []string{teamA, teamB}).Scan(&playerTeam)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusForbidden, "Only a player on one of the fixture teams can veto")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if playerTeam != turnTeam {
		fail(w, http.StatusConflict, "It is the other team’s turn")
		return
	}
	if step == 6 {
		if in.Side != "ct" && in.Side != "t" {
			fail(w, http.StatusBadRequest, "Choose CT or T for your team’s starting side")
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET team_b_side=$2,veto_step=7,veto_turn_team_id=NULL WHERE id=$1`, r.PathValue("match"), in.Side); err != nil {
			internal(w, err)
			return
		}
	} else {
		isPick := tournamentVetoPick(bestOf, step)
		if in.Map == "" || in.Side != "" {
			verb := "ban"
			if isPick {
				verb = "pick"
			}
			fail(w, http.StatusBadRequest, "Choose one map to "+verb)
			return
		}
		mapInPool := false
		for _, mapName := range mapPool {
			if mapName == in.Map {
				mapInPool = true
				break
			}
		}
		if !mapInPool {
			fail(w, http.StatusBadRequest, "Choose a map from this tournament’s map pool")
			return
		}
		for _, used := range append(append([]string{}, bans...), picks...) {
			if used == in.Map {
				fail(w, http.StatusConflict, "That map has already been used in the veto")
				return
			}
		}
		if isPick {
			picks = append(picks, in.Map)
			sequence = append(sequence, in.Map)
		} else {
			bans = append(bans, in.Map)
		}
		nextStep := step + 1
		nextTurn := tournamentVetoTurn(bestOf, nextStep, teamA, teamB)
		var selectedMap any
		if nextStep == 6 {
			for _, mapName := range mapPool {
				used := false
				for _, candidate := range append(append([]string{}, bans...), picks...) {
					if candidate == mapName {
						used = true
						break
					}
				}
				if !used {
					if selectedMap != nil {
						fail(w, http.StatusConflict, "Map veto state is inconsistent; contact an administrator")
						return
					}
					selectedMap = mapName
				}
			}
			if selectedMap == nil {
				fail(w, http.StatusConflict, "Map veto state is inconsistent; contact an administrator")
				return
			}
			sequence = append(sequence, selectedMap.(string))
			selectedMap = sequence[0]
		}
		if _, err = tx.Exec(ctx, `UPDATE tournament_matches SET veto_bans=$2,veto_maps=$3,map_sequence=$4,veto_step=$5,veto_turn_team_id=$6,selected_map=COALESCE($7,selected_map) WHERE id=$1`, r.PathValue("match"), bans, picks, sequence, nextStep, nextTurn, selectedMap); err != nil {
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
