package control

import (
	"context"
	"testing"
)

func TestTournamentResultRevisionTracksFixtureChanges(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO tournaments(id,name,players_per_side,status) VALUES('revision-cup','Cup',1,'active'); INSERT INTO tournament_matches(id,tournament_id,round,number,status) VALUES('revision-fixture','revision-cup',1,1,'pending')`); err != nil {
		t.Fatal(err)
	}
	var revision int64
	check := func(want int64) {
		t.Helper()
		if err := a.DB.QueryRow(ctx, `SELECT result_revision FROM tournament_matches WHERE id='revision-fixture'`).Scan(&revision); err != nil || revision != want {
			t.Fatalf("revision %d, want %d: %v", revision, want, err)
		}
	}
	check(0)
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET veto_step=0 WHERE id='revision-fixture'`); err != nil {
		t.Fatal(err)
	}
	check(0)
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET status='bye' WHERE id='revision-fixture'`); err != nil {
		t.Fatal(err)
	}
	check(1)
	if _, err := a.DB.Exec(ctx, `UPDATE tournament_matches SET status='pending' WHERE id='revision-fixture'`); err != nil {
		t.Fatal(err)
	}
	check(2)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO teams(id,name,captain_steam_id) VALUES('revision-a','A','76561198154367261'),('revision-b','B','76561198154367262'); UPDATE tournament_matches SET team_a='revision-a',team_b='revision-b',winner_team_id='revision-a',status='complete' WHERE id='revision-fixture'; UPDATE tournament_matches SET winner_team_id='revision-b' WHERE id='revision-fixture'`); err != nil {
		t.Fatal(err)
	}
	var oldWinner, newWinner string
	var historyRevision int64
	if err := a.DB.QueryRow(ctx, `SELECT previous_winner,new_winner,revision FROM tournament_winner_history WHERE match_id='revision-fixture' AND previous_winner IS NOT NULL`).Scan(&oldWinner, &newWinner, &historyRevision); err != nil || oldWinner != "revision-a" || newWinner != "revision-b" || historyRevision != 4 {
		t.Fatalf("winner history: %s -> %s revision %d: %v", oldWinner, newWinner, historyRevision, err)
	}
}
