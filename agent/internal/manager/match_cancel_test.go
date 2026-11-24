package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func TestMatchCancellationReplayKeepsItsTarget(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	if err := os.MkdirAll(m.accessDir("server"), 0700); err != nil {
		t.Fatal(err)
	}
	command := protocol.MatchCancel{ID: "ABCDEFGHIJKLMNOPQRSTUVWX23", SetupID: "BCDEFGHIJKLMNOPQRSTUVWXY23"}
	path := filepath.Join(m.accessDir("server"), "match-cancel-request.json")
	if err := access.Write(path, access.MatchCancelRequest{ID: command.ID, SetupID: command.SetupID}); err != nil {
		t.Fatal(err)
	}
	if err := m.RequestMatchCancellation(context.Background(), "server", command); err != nil {
		t.Fatal(err)
	}
	command.SetupID = "CDEFGHIJKLMNOPQRSTUVWXYZ23"
	if err := m.RequestMatchCancellation(context.Background(), "server", command); err == nil {
		t.Fatal("reused cancellation ID with a different setup")
	}
	command.ID = "invalid"
	if err := m.RequestMatchCancellation(context.Background(), "server", command); err == nil {
		t.Fatal("invalid command ID accepted")
	}
}
