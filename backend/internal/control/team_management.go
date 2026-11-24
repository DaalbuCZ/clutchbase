package control

import (
	"crypto/rand"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

const teamRosterLockedSQL = `EXISTS(SELECT 1 FROM tournament_entries e JOIN tournaments cup ON cup.id=e.tournament_id WHERE e.team_id=t.id AND e.status='checked_in' AND cup.status IN ('open','checkin','active'))`
const teamRegisteredSQL = `EXISTS(SELECT 1 FROM tournament_entries e JOIN tournaments cup ON cup.id=e.tournament_id WHERE e.team_id=t.id AND cup.status IN ('open','checkin','active'))`

func (a *API) manageTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Revision *int64 `json:"revision"`
		SteamID  string `json:"steamId"`
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
	var revision int64
	var locked, registered bool
	err = tx.QueryRow(ctx, `SELECT t.revision FROM teams t WHERE t.id=$1 AND t.captain_steam_id=$2 AND t.disbanded_at IS NULL FOR UPDATE`, r.PathValue("id"), p.SteamID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Team not found or captain access required")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, `SELECT (`+teamRosterLockedSQL+`),(`+teamRegisteredSQL+`) FROM teams t WHERE t.id=$1`, r.PathValue("id")).Scan(&locked, &registered); err != nil {
		internal(w, err)
		return
	}
	if in.Revision == nil || *in.Revision != revision {
		fail(w, 409, "The team changed; wait for its latest snapshot")
		return
	}
	if locked {
		fail(w, 409, "The team roster is locked while checked in to a tournament")
		return
	}
	target := r.PathValue("steam")
	if r.Method == http.MethodPatch {
		target = in.SteamID
	}
	if target != "" {
		var member bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id=$1 AND steam_id=$2)`, r.PathValue("id"), target).Scan(&member); err != nil {
			internal(w, err)
			return
		}
		if !member || target == p.SteamID {
			fail(w, 409, "Choose another current team member")
			return
		}
		if r.Method == http.MethodPatch {
			_, err = tx.Exec(ctx, `UPDATE teams SET captain_steam_id=$2,revision=revision+1 WHERE id=$1`, r.PathValue("id"), target)
		} else {
			if _, err = tx.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1 AND steam_id=$2`, r.PathValue("id"), target); err == nil {
				_, err = tx.Exec(ctx, `UPDATE teams SET revision=revision+1 WHERE id=$1`, r.PathValue("id"))
			}
		}
	} else {
		if r.Method != http.MethodDelete {
			fail(w, 400, "Choose a current team member")
			return
		}
		if registered {
			fail(w, 409, "Finish the team's tournament registrations before disbanding")
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE teams SET disbanded_at=now(),revision=revision+1 WHERE id=$1`, r.PathValue("id")); err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1`, r.PathValue("id"))
		}
	}
	if err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM team_invites WHERE team_id=$1`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) inviteTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
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
	var locked bool
	var size int
	var team string
	err = tx.QueryRow(ctx, `SELECT id FROM teams t WHERE t.id=$1 AND t.captain_steam_id=$2 AND t.disbanded_at IS NULL FOR UPDATE`, r.PathValue("id"), p.SteamID).Scan(&team)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Team not found or captain access required")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.QueryRow(ctx, `SELECT (`+teamRosterLockedSQL+`),(SELECT count(*) FROM team_members WHERE team_id=t.id) FROM teams t WHERE t.id=$1`, team).Scan(&locked, &size); err != nil {
		internal(w, err)
		return
	}
	if locked || size >= 5 {
		fail(w, 409, "The team is full or its tournament roster is locked")
		return
	}
	code := rand.Text() + rand.Text()
	if _, err = tx.Exec(ctx, `INSERT INTO team_invites(team_id,code_hash,expires_at) VALUES($1,$2,now()+interval '24 hours') ON CONFLICT(team_id) DO UPDATE SET code_hash=EXCLUDED.code_hash,expires_at=EXCLUDED.expires_at`, r.PathValue("id"), digest(code)); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]string{"code": code})
}
