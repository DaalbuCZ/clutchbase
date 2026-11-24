package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestRoundRobinSchedule(t *testing.T) {
	for n := 2; n <= 32; n++ {
		ids := []string{}
		for i := 0; i < n; i++ {
			ids = append(ids, fmt.Sprint(i))
		}
		fixtures := roundRobinFixtures(ids)
		if len(fixtures) != n*(n-1)/2 {
			t.Fatalf("%d teams: %d fixtures", n, len(fixtures))
		}
		pairs := map[string]bool{}
		rounds := map[int]map[string]bool{}
		for _, f := range fixtures {
			a, b := f.a, f.b
			if a > b {
				a, b = b, a
			}
			key := a + ":" + b
			if a == b || pairs[key] {
				t.Fatalf("repeated/invalid pair: %+v", f)
			}
			pairs[key] = true
			if rounds[f.round] == nil {
				rounds[f.round] = map[string]bool{}
			}
			if rounds[f.round][a] || rounds[f.round][b] {
				t.Fatalf("team plays twice in round: %+v", f)
			}
			rounds[f.round][a], rounds[f.round][b] = true, true
		}
	}
}

func TestRoundRobinAPIAndCompletion(t *testing.T) {
	for _, tied := range []bool{false, true} {
		t.Run(fmt.Sprintf("tied=%v", tied), func(t *testing.T) {
			a := integrationAPI(t)
			ctx := context.Background()
			call := func(path string, body any, admin bool) *httptest.ResponseRecorder {
				data, _ := json.Marshal(body)
				r := httptest.NewRequest("POST", path, bytes.NewReader(data))
				r.Header.Set("Origin", a.Origin)
				if admin {
					r.Header.Set("Authorization", "Bearer "+a.AdminToken)
				}
				w := httptest.NewRecorder()
				a.Handler().ServeHTTP(w, r)
				return w
			}
			input := map[string]any{"name": "Friends league", "playersPerSide": 1, "format": "round_robin", "bestOf": 3}
			if w := call("/api/tournaments", input, false); w.Code != 401 {
				t.Fatalf("unauthorized create: %d", w.Code)
			}
			bad := map[string]any{"name": "Invalid", "playersPerSide": 1, "format": "unknown"}
			if w := call("/api/tournaments", bad, true); w.Code != 400 {
				t.Fatalf("invalid format: %d %s", w.Code, w.Body.String())
			}
			w := call("/api/tournaments", input, true)
			if w.Code != 201 {
				t.Fatal(w.Code, w.Body.String())
			}
			var created struct{ ID string }
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			for i, id := range []string{"a", "b", "c"} {
				steam := fmt.Sprintf("7656119815436726%d", i)
				if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,$2);`, steam, id); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO teams(id,name,captain_steam_id) VALUES($1,$1,$2)`, id, steam); err != nil {
					t.Fatal(err)
				}
				if _, err := a.DB.Exec(ctx, `INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES($1,$2,'checked_in')`, created.ID, id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := a.DB.Exec(ctx, `UPDATE tournaments SET status='checkin' WHERE id=$1`, created.ID); err != nil {
				t.Fatal(err)
			}
			path := "/api/tournaments/" + created.ID + "/bracket"
			if w := call(path, nil, false); w.Code != 401 {
				t.Fatalf("unauthorized fixture generation: %d", w.Code)
			}
			if w := call(path, nil, true); w.Code != 204 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w := call(path, nil, true); w.Code != 409 {
				t.Fatalf("generated fixtures twice: %d", w.Code)
			}
			ts, err := a.tournaments(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cup := ts[0]
			if cup.Format != "round_robin" || len(cup.Matches) != 3 || len(cup.Standings) != 3 || cup.BestOf != 3 {
				t.Fatalf("bad snapshot: %+v", cup)
			}
			future := cup.Matches[len(cup.Matches)-1]
			if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=7,selected_map='de_mirage',team_b_side='t' WHERE id=$1`, future.ID); err != nil {
				t.Fatal(err)
			}
			if w := call("/api/tournaments/"+created.ID+"/matches/"+future.ID+"/room", map[string]string{"hostId": "missing"}, true); w.Code != 409 {
				t.Fatalf("future round opened: %d %s", w.Code, w.Body.String())
			}
			blocked, err := a.tournaments(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if blocked[0].Matches[len(cup.Matches)-1].CanOpenRoom {
				t.Fatal("future fixture offered a room before previous round completed")
			}
			for i, m := range cup.Matches {
				winner := m.TeamA.TeamID
				if tied { // A beats B, B beats C, C beats A.
					aID, bID := m.TeamA.TeamID, m.TeamB.TeamID
					if (aID == "a" && bID == "c") || (aID == "b" && bID == "a") || (aID == "c" && bID == "b") {
						winner = bID
					}
				} else {
					if m.TeamA.TeamID == "a" || m.TeamB.TeamID == "a" {
						winner = "a"
					} else {
						winner = "b"
					}
				}
				if i == len(cup.Matches)-1 {
					exec := func(sql string, args ...any) {
						t.Helper()
						if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
							t.Fatal(err)
						}
					}
					exec(`INSERT INTO hosts(id,name,address,port,token_hash,map) VALUES('host','Host','localhost',27015,'','de_mirage')`)
					first, second := 1, 2
					if winner == m.TeamA.TeamID {
						first, second = 2, 1
					}
					exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id,load_fence_required,reported_ct,reported_t,reported_at,reported_team1_score,reported_team2_score) VALUES('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage','closed','complete','BCDEFGHIJKLMNOPQRSTUVWXY23',true,13,9,now(),$1,$2)`, first, second)
					exec(`UPDATE tournament_matches SET room_id='ABCDEFGHIJKLMNOPQRSTUVWX23',team_b_side='t' WHERE id=$1`, m.ID)
					if err := a.advanceTournamentFromReportedResult(ctx, "host", "BCDEFGHIJKLMNOPQRSTUVWXY23"); err != nil {
						t.Fatal(err)
					}
					// A retry of the same result must not count as another win.
					if err := a.advanceTournamentFromReportedResult(ctx, "host", "BCDEFGHIJKLMNOPQRSTUVWXY23"); err != nil {
						t.Fatal(err)
					}
					continue
				}
				tx, err := a.DB.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err = advanceTournamentWinner(ctx, tx, created.ID, m.ID, m.Round, m.Number, winner); err != nil {
					tx.Rollback(ctx)
					t.Fatal(err)
				}
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				var status string
				if err = a.DB.QueryRow(ctx, `SELECT status FROM tournaments WHERE id=$1`, created.ID).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if i < 2 && status != "active" {
					t.Fatal("completed before all fixtures")
				}
			}
			ts, err = a.tournaments(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cup = ts[0]
			if cup.Status != "complete" {
				t.Fatal("not completed")
			}
			if tied {
				if cup.WinnerTeamID != "" {
					t.Fatal("arbitrary tie winner")
				}
				for _, s := range cup.Standings {
					if s.Rank != 1 || s.Played != 2 || s.Wins != 1 || s.Losses != 1 {
						t.Fatalf("bad shared rank: %+v", s)
					}
				}
			} else if cup.WinnerTeamID != "a" || cup.Standings[0].TeamID != "a" || cup.Standings[0].Wins != 2 {
				t.Fatalf("wrong champion: %+v", cup)
			}
			public := projectPlayerTournaments(ts, "outsider", nil)[0]
			if len(public.Standings) != 3 || public.CanBuildBracket || public.CanCheckIn {
				t.Fatalf("public standings/actions: %+v", public)
			}
		})
	}
}
