package manager

import (
	"os"
	"path/filepath"
	"testing"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func TestFinishedResultsRetryUntilMatchingAcknowledgement(t *testing.T) {
	m, err := New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := m.accessDir("server")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	team1, team2 := 2, 1
	result := protocol.MatchResult{SetupID: "ABCDEFGHIJKLMNOPQRSTUVWX23", CT: 13, T: 9, Team1Score: &team1, Team2Score: &team2, Players: []protocol.ReportedPlayer{{SteamID: "76561198154367261", Kills: 10, Deaths: 5, Assists: 2}}}
	path := filepath.Join(dir, "result-finished-"+result.SetupID+".json")
	wire := struct {
		SetupID    string                    `json:"setupId"`
		CT         int                       `json:"ct"`
		T          int                       `json:"t"`
		ScoreCT    int                       `json:"scoreCt"`
		ScoreT     int                       `json:"scoreT"`
		Team1Score *int                      `json:"team1Score"`
		Team2Score *int                      `json:"team2Score"`
		Players    []protocol.ReportedPlayer `json:"players"`
	}{result.SetupID, -1, -1, result.CT, result.T, result.Team1Score, result.Team2Score, result.Players}
	if err := access.Write(path, wire); err != nil {
		t.Fatal(err)
	}
	var legacy protocol.MatchResult
	if err := access.Read(path, &legacy); err != nil || legacy.Valid() {
		t.Fatal("older agents could acknowledge an incomplete result")
	}
	if got := m.PendingMatchResult("server", false); got != nil {
		t.Fatal("sent stats to an older API")
	}
	if got := m.PendingMatchResult("server", true); got == nil || !got.Equal(result) {
		t.Fatalf("missing pending: %+v", got)
	}
	wrong := result
	wrong.T = 8
	if m.AcknowledgeMatchResult("server", wrong) == nil {
		t.Fatal("accepted wrong score")
	}
	wrong = result
	wrong.Players = []protocol.ReportedPlayer{{SteamID: "76561198154367261", Kills: 11, Deaths: 5, Assists: 2}}
	if m.AcknowledgeMatchResult("server", wrong) == nil {
		t.Fatal("accepted changed player stats")
	}
	if got := m.PendingMatchResult("server", true); got == nil || !got.Equal(result) {
		t.Fatal("wrong acknowledgement lost result")
	}
	if err := m.AcknowledgeMatchResult("server", result); err != nil {
		t.Fatal(err)
	}
	if got := m.PendingMatchResult("server", true); got != nil {
		t.Fatalf("delivered result replayed: %+v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("original result was not retained", err)
	}
	if err := os.Remove(filepath.Join(dir, "result-ack-"+result.SetupID+".json")); err != nil {
		t.Fatal(err)
	}
	if got := m.PendingMatchResult("server", true); got == nil || !got.Equal(result) {
		t.Fatal("lost acknowledgement did not retry")
	}
	if m.AcknowledgeMatchResult("../bad", result) == nil {
		t.Fatal("accepted invalid instance")
	}
}
