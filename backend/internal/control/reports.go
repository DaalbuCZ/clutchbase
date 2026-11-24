package control

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type matchReport struct {
	ID              string    `json:"id"`
	MatchID         string    `json:"matchId"`
	MatchTitle      string    `json:"matchTitle"`
	HostName        string    `json:"hostName"`
	ReporterSteamID string    `json:"reporterSteamId,omitempty"`
	ReporterName    string    `json:"reporterName,omitempty"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	Revision        int64     `json:"revision"`
	CreatedAt       time.Time `json:"createdAt"`
}
type reportPage struct {
	CapturedAt string        `json:"capturedAt"`
	Reports    []matchReport `json:"reports"`
	Truncated  bool          `json:"truncated"`
}

func (a *API) matchReports(ctx context.Context, steam string) (reportPage, error) {
	page := reportPage{CapturedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Reports: []matchReport{}}
	rows, err := a.DB.Query(ctx, `SELECT r.id,r.match_id,m.title,h.name,CASE WHEN $1='' THEN r.reporter_steam_id ELSE '' END,CASE WHEN $1='' THEN COALESCE(NULLIF(p.game_name,''),'Player '||right(p.steam_id,4)) ELSE '' END,r.description,r.status,r.revision,r.created_at FROM match_reports r JOIN match_rooms m ON m.id=r.match_id JOIN hosts h ON h.id=m.host_id JOIN players p ON p.steam_id=r.reporter_steam_id WHERE $1='' OR r.reporter_steam_id=$1 ORDER BY (r.status='open') DESC,r.created_at DESC,r.id DESC LIMIT 101`, steam)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var v matchReport
		if err := rows.Scan(&v.ID, &v.MatchID, &v.MatchTitle, &v.HostName, &v.ReporterSteamID, &v.ReporterName, &v.Description, &v.Status, &v.Revision, &v.CreatedAt); err != nil {
			return page, err
		}
		page.Reports = append(page.Reports, v)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Reports) > 100 {
		page.Reports = page.Reports[:100]
		page.Truncated = true
	}
	return page, nil
}

func (a *API) submitMatchReport(w http.ResponseWriter, r *http.Request) {
	player, ok := a.requirePlayerMutation(w, r)
	if !ok {
		return
	}
	var in struct {
		Description string `json:"description"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	if !utf8.ValidString(in.Description) || utf8.RuneCountInString(in.Description) < 1 || utf8.RuneCountInString(in.Description) > 1000 || strings.ContainsRune(in.Description, 0) {
		fail(w, 400, "Describe the issue in 1 to 1000 characters")
		return
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var host string
	err = tx.QueryRow(ctx, `SELECT h.id FROM hosts h JOIN match_rooms m ON m.host_id=h.id WHERE m.id=$1 AND h.removed_at IS NULL AND $2=ANY(h.whitelist) FOR UPDATE OF h`, r.PathValue("id"), player.SteamID).Scan(&host)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Match not available")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var lockedRoom string
	if err = tx.QueryRow(ctx, `SELECT id FROM match_rooms WHERE id=$1 FOR UPDATE`, r.PathValue("id")).Scan(&lockedRoom); err != nil {
		internal(w, err)
		return
	}
	var joined bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM match_room_players WHERE match_id=$1 AND steam_id=$2)`, r.PathValue("id"), player.SteamID).Scan(&joined); err != nil {
		internal(w, err)
		return
	}
	if !joined {
		fail(w, 403, "Only match participants can submit a report")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO match_reports(id,match_id,reporter_steam_id,description) VALUES($1,$2,$3,$4)`, rand.Text(), r.PathValue("id"), player.SteamID, in.Description)
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			fail(w, 409, "You already reported this match")
			return
		}
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (a *API) reviewMatchReport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status   string `json:"status"`
		Revision *int64 `json:"revision"`
	}
	if !decode(w, r, &in) {
		return
	}
	if (in.Status != "open" && in.Status != "resolved") || in.Revision == nil || *in.Revision < 1 {
		fail(w, 400, "Choose a report status and current revision")
		return
	}
	result, err := a.DB.Exec(r.Context(), `UPDATE match_reports SET status=$2,revision=revision+1 WHERE id=$1 AND revision=$3 AND status<>$2`, r.PathValue("id"), in.Status, *in.Revision)
	if err != nil {
		internal(w, err)
		return
	}
	if result.RowsAffected() != 1 {
		fail(w, 409, "Report changed; wait for the current snapshot")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
