package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTournamentCancellationKeepsHistoryAndReleasesOnlyItsLocks(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262')`)
	exec(`INSERT INTO teams(id,name,captain_steam_id) VALUES('a','Alpha','76561198154367261'),('b','Bravo','76561198154367262')`)
	exec(`INSERT INTO team_members(team_id,steam_id) VALUES('a','76561198154367261'),('b','76561198154367262')`)
	exec(`INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'checkin'),('other','Other cup',1,'checkin')`)
	exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','checked_in'),('other','a','checked_in')`)
	call := func(path, body, token string, want int) {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
	}
	call("/api/tournaments/cup/cancel", `{"phase":"checkin"}`, "", 401)
	call("/api/tournaments/cup/cancel", `{"phase":"active"}`, a.AdminToken, 400)
	call("/api/tournaments/cup/cancel", `{"phase":"open"}`, a.AdminToken, 409)
	before, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, cup := range before {
		if !cup.CanCancel {
			t.Fatal("prestart tournament cancellation unavailable")
		}
	}
	call("/api/tournaments/cup/cancel", `{"phase":"checkin"}`, a.AdminToken, 204)
	alpha, err := a.playerTeam(ctx, "76561198154367261")
	if err != nil || !alpha.RosterLocked {
		t.Fatalf("another registration lost its lock: %+v %v", alpha, err)
	}
	bravo, err := a.playerTeam(ctx, "76561198154367262")
	if err != nil || bravo.RosterLocked || bravo.Registered || !bravo.Actions.Disband {
		t.Fatalf("cancelled tournament retained roster lock: %+v %v", bravo, err)
	}
	cups, err := a.tournaments(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, cup := range cups {
		if cup.ID == "cup" {
			if cup.Status != "cancelled" || len(cup.Entries) != 2 || cup.CanCancel || cup.CanBuildBracket || cup.CanCheckIn {
				t.Fatalf("bad cancelled snapshot: %+v", cup)
			}
		}
	}
	public := projectPlayerTournaments(cups, "76561198154367262", bravo)
	for _, cup := range public {
		if cup.CanCancel {
			t.Fatal("player received administrator action")
		}
		if cup.ID == "cup" && (cup.CanWithdrawTeam || cup.CanCheckInTeam || cup.CanRegisterTeam) {
			t.Fatal("cancelled cup offered player actions")
		}
	}
	call("/api/tournaments/cup/cancel", `{"phase":"checkin"}`, a.AdminToken, 409)
	call("/api/tournaments/cup/bracket", `{}`, a.AdminToken, 409)
	exec(`INSERT INTO tournament_matches(id,tournament_id,round,number,status,team_a,team_b) VALUES('fixture','other',1,1,'pending','a','b')`)
	call("/api/tournaments/other/cancel", `{"phase":"checkin"}`, a.AdminToken, 409)
}

func TestCancellationAndSeedingSerialize(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	for i, id := range []string{"76561198154367261", "76561198154367262"} {
		team := string(rune('a' + i))
		if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, team, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status) VALUES('cup','Cup',1,'checkin');INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES('cup','a','checked_in'),('cup','b','checked_in')`); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	cancelled := make(chan int, 1)
	seeded := make(chan int, 1)
	for _, op := range []struct {
		path, body string
		result     chan int
	}{{"cancel", `{"phase":"checkin"}`, cancelled}, {"bracket", `{}`, seeded}} {
		go func(path, body string, result chan int) {
			<-start
			r := httptest.NewRequest("POST", "/api/tournaments/cup/"+path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer "+a.AdminToken)
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			result <- w.Code
		}(op.path, op.body, op.result)
	}
	close(start)
	c, s := <-cancelled, <-seeded
	if (c != 204 || s != 409) && (c != 409 || s != 204) {
		t.Fatalf("cancellation=%d generation=%d", c, s)
	}
	var phase string
	var fixtures int
	if err := a.DB.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM tournament_matches WHERE tournament_id='cup') FROM tournaments WHERE id='cup'`).Scan(&phase, &fixtures); err != nil {
		t.Fatal(err)
	}
	if (phase == "cancelled" && fixtures != 0) || (phase == "active" && fixtures != 1) {
		t.Fatalf("cancellation and fixtures both committed: %s %d", phase, fixtures)
	}
}
