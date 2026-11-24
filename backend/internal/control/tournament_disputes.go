package control

import (
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

func (a *API) submitTournamentDispute(w http.ResponseWriter, r *http.Request) {
	p, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) == 0 || len(in.Reason) > 2000 {
		fail(w, 400, "Reason must be between 1 and 2000 bytes")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var participant, eligible bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_room_players p WHERE p.match_id=m.room_id AND p.steam_id=$3), COALESCE(r.status='closed' AND r.load_status='complete' AND ((r.reported_ct IS NOT NULL AND r.reported_t IS NOT NULL) OR (r.reported_team1_score IS NOT NULL AND r.reported_team2_score IS NOT NULL)),false) FROM tournament_matches m LEFT JOIN match_rooms r ON r.id=m.room_id WHERE m.tournament_id=$1 AND m.id=$2 FOR UPDATE OF m`, r.PathValue("id"), r.PathValue("match"), p.SteamID).Scan(&participant, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Tournament fixture not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !participant {
		fail(w, 403, "Only fixture participants can submit a dispute")
		return
	}
	if !eligible {
		fail(w, 409, "This fixture has no reported result to dispute")
		return
	}
	tag, err := tx.Exec(ctx, `INSERT INTO tournament_disputes(match_id,reason) VALUES($1,$2) ON CONFLICT DO NOTHING`, r.PathValue("match"), in.Reason)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "This fixture already has a dispute")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *API) resolveTournamentDispute(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Revision   *int64 `json:"revision"`
		Resolution string `json:"resolution"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Resolution = strings.TrimSpace(in.Resolution)
	if in.Revision == nil || *in.Revision < 0 || len(in.Resolution) == 0 || len(in.Resolution) > 2000 {
		fail(w, 400, "Revision and a resolution of 1 to 2000 bytes are required")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE tournament_disputes d SET resolution=$4,revision=revision+1 FROM tournament_matches m WHERE d.match_id=m.id AND m.tournament_id=$1 AND m.id=$2 AND d.revision=$3 AND d.resolution=''`, r.PathValue("id"), r.PathValue("match"), *in.Revision, in.Resolution)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 409, "Dispute changed or is already resolved")
		return
	}
	w.WriteHeader(204)
}
