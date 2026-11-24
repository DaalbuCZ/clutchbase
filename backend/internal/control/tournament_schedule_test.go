package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTournamentScheduleAdminRevisionAndPublicProjection(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'active')`,
		`INSERT INTO tournament_matches(id,tournament_id,round,number,status) VALUES('pending','cup',1,1,'pending'),('done','cup',1,2,'complete')`,
	} {
		if _, err := a.DB.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	call := func(id, body, token string, want int) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/tournaments/cup/matches/"+id+"/schedule", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s returned %d want %d: %s", id, w.Code, want, w.Body.String())
		}
	}
	call("pending", `{"revision":0,"scheduledAt":"2026-12-01T18:00:00Z"}`, "", 401)
	call("pending", `{"revision":0,"scheduledAt":"tomorrow"}`, a.AdminToken, 400)
	call("pending", `{}`, a.AdminToken, 400)
	call("pending", `{"revision":0}`, a.AdminToken, 400)
	call("pending", `{"revision":0,"scheduledAt":"2026-12-01T18:00:00Z"}`, a.AdminToken, 204)
	call("pending", `{"revision":0,"scheduledAt":null}`, a.AdminToken, 409)
	call("pending", `{"revision":1,"scheduledAt":null}`, a.AdminToken, 204)
	call("done", `{"revision":0,"scheduledAt":null}`, a.AdminToken, 409)
	ts, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || len(ts[0].Matches) != 2 {
		t.Fatalf("unexpected projection: %+v", ts)
	}
	if ts[0].Matches[0].ScheduleRevision != 2 || ts[0].Matches[0].ScheduledAt != nil {
		t.Fatalf("clear not projected: %+v", ts[0].Matches[0])
	}
	if !ts[0].Matches[0].CanSchedule || ts[0].Matches[1].CanSchedule {
		t.Fatal("admin actions have wrong state")
	}
	public := projectPlayerTournaments(ts, "outsider", nil)
	if public[0].Matches[0].CanSchedule {
		t.Fatal("public projection exposed admin schedule action")
	}
}
