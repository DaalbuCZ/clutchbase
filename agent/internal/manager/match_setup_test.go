package manager

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func TestMatchSetupReplayRequiresIdenticalPayload(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	name := "server"
	if err := os.MkdirAll(m.accessDir(name), 0700); err != nil {
		t.Fatal(err)
	}
	setup := protocol.MatchSetup{ID: rand.Text(), Overtime: true, Map: "de_nuke", PlayersPerSide: 1, CT: []protocol.MatchSetupPlayer{{SteamID: "76561198154367261", GameName: "CT player"}}, T: []protocol.MatchSetupPlayer{{SteamID: "76561198154367262", GameName: "T player"}}}
	path := filepath.Join(m.accessDir(name), "match-request.json")
	if err := access.Write(path, access.MatchSetupRequest{ID: setup.ID, Map: setup.Map, Overtime: setup.Overtime, PlayersPerSide: setup.PlayersPerSide, CT: setup.CT, T: setup.T, RequestedAt: 1234}); err != nil {
		t.Fatal(err)
	}
	// The game may already have entered a match; replaying the same request must
	// not recheck availability, overwrite the request, or create an agent error.
	if err := m.RequestMatchSetup(context.Background(), name, setup); err != nil {
		t.Fatal(err)
	}
	var saved access.MatchSetupRequest
	if err := access.Read(path, &saved); err != nil || saved.RequestedAt != 1234 {
		t.Fatalf("request was rewritten: %+v %v", saved, err)
	}
	changed := setup
	changed.CT = append([]protocol.MatchSetupPlayer(nil), setup.CT...)
	changed.CT[0].GameName = "Changed name"
	if err := m.RequestMatchSetup(context.Background(), name, changed); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("accepted altered payload: %v", err)
	}
	changed = setup
	changed.T = []protocol.MatchSetupPlayer{setup.CT[0]}
	if err := m.RequestMatchSetup(context.Background(), name, changed); err == nil {
		t.Fatal("accepted one player on both sides")
	}
	changed = setup
	changed.Overtime = false
	if err := m.RequestMatchSetup(context.Background(), name, changed); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("accepted changed overtime: %v", err)
	}
	changed = setup
	changed.FenceRequired = true
	if err := m.RequestMatchSetup(context.Background(), name, changed); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("accepted a changed fence requirement: %v", err)
	}
	changed = setup
	changed.ManualReady = true
	if err := m.RequestMatchSetup(context.Background(), name, changed); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("accepted a changed manual-ready setting: %v", err)
	}
}

func TestMatchSetupRejectsDelayedOrChangedTimestamp(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	name := "server"
	if err := os.MkdirAll(m.accessDir(name), 0700); err != nil {
		t.Fatal(err)
	}
	setup := protocol.MatchSetup{ID: rand.Text(), RequestedAt: time.Now().Add(-61 * time.Second).UnixMilli(), Map: "de_nuke", PlayersPerSide: 1, CT: []protocol.MatchSetupPlayer{{SteamID: "76561198154367261", GameName: "CT"}}, T: []protocol.MatchSetupPlayer{{SteamID: "76561198154367262", GameName: "T"}}}
	path := filepath.Join(m.accessDir(name), "match-request.json")
	if err := m.RequestMatchSetup(context.Background(), name, setup); err == nil {
		t.Fatal("accepted a delayed request")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("delayed request wrote a file: %v", err)
	}
	setup.RequestedAt = time.Now().UnixMilli()
	if err := access.Write(path, access.MatchSetupRequest{ID: setup.ID, Map: setup.Map, PlayersPerSide: setup.PlayersPerSide, CT: setup.CT, T: setup.T, RequestedAt: setup.RequestedAt}); err != nil {
		t.Fatal(err)
	}
	if err := m.RequestMatchSetup(context.Background(), name, setup); err != nil {
		t.Fatal(err)
	}
	setup.RequestedAt++
	if err := m.RequestMatchSetup(context.Background(), name, setup); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("accepted a changed timestamp: %v", err)
	}
	setup.RequestedAt = time.Now().Add(10 * time.Second).UnixMilli()
	if err := m.RequestMatchSetup(context.Background(), name, setup); err == nil {
		t.Fatal("accepted an unsafe clock difference")
	}
}
