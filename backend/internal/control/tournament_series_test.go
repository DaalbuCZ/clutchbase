package control

import (
	"context"
	"testing"
)

func TestReportedBestOfThreeAdvancesCorrectTeamForEitherStartingSide(t *testing.T) {
	for _, tc := range []struct{ teamBSide, want string }{{"ct", "team-b"}, {"t", "team-a"}} {
		t.Run(tc.teamBSide, func(t *testing.T) {
			a := integrationAPI(t)
			ctx := context.Background()
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			exec(`INSERT INTO players(steam_id) VALUES ('76561198154367261'),('76561198154367262'),('76561198154367263')`)
			exec(`INSERT INTO teams(id,name,captain_steam_id) VALUES ('team-a','Alpha','76561198154367261'),('team-b','Bravo','76561198154367262'),('team-c','Charlie','76561198154367263')`)
			exec(`INSERT INTO tournaments(id,name,players_per_side,status,map_pool,best_of) VALUES ('cup','Cup',1,'active',ARRAY['de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train'],3)`)
			exec(`INSERT INTO tournament_entries(tournament_id,team_id,status) VALUES ('cup','team-a','checked_in'),('cup','team-b','checked_in'),('cup','team-c','checked_in')`)
			exec(`INSERT INTO hosts(id,name,address,port,token_hash,map) VALUES ('host','Host','localhost',27015,'','de_mirage')`)
			exec(`INSERT INTO match_rooms(id,host_id,title,map,status,load_status,load_command_id,load_fence_required,reported_ct,reported_t,reported_at,reported_team1_score,reported_team2_score) VALUES ('ABCDEFGHIJKLMNOPQRSTUVWX23','host','Room','de_mirage','closed','complete','BCDEFGHIJKLMNOPQRSTUVWXY23',true,13,9,now(),2,1)`)
			exec(`INSERT INTO tournament_matches(id,tournament_id,round,number,team_a,team_b,status,room_id,team_b_side) VALUES ('fixture','cup',1,1,'team-a','team-b','pending','ABCDEFGHIJKLMNOPQRSTUVWX23',$1)`, tc.teamBSide)
			exec(`INSERT INTO tournament_matches(id,tournament_id,round,number,team_b,status) VALUES ('final','cup',2,1,'team-c','pending')`)
			if err := a.advanceTournamentFromReportedResult(ctx, "host", "BCDEFGHIJKLMNOPQRSTUVWXY23"); err != nil {
				t.Fatal(err)
			}
			var winner, next string
			if err := a.DB.QueryRow(ctx, `SELECT winner_team_id FROM tournament_matches WHERE id='fixture'`).Scan(&winner); err != nil {
				t.Fatal(err)
			}
			if err := a.DB.QueryRow(ctx, `SELECT team_a FROM tournament_matches WHERE id='final'`).Scan(&next); err != nil {
				t.Fatal(err)
			}
			if winner != tc.want || next != tc.want {
				t.Fatalf("advanced winner=%q next=%q, want %q", winner, next, tc.want)
			}
		})
	}
}
