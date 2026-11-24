package manager

import (
	"context"
	"errors"
	"path/filepath"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func (m *Manager) RequestMatchCancellation(ctx context.Context, name string, command protocol.MatchCancel) error {
	if !protocol.ValidCommandID(command.ID) || !protocol.ValidCommandID(command.SetupID) {
		return errors.New("invalid match cancellation")
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(m.accessDir(name), "match-cancel-request.json")
	var previous access.MatchCancelRequest
	if access.Read(path, &previous) == nil && previous.ID == command.ID {
		if previous.SetupID != command.SetupID {
			return errors.New("match cancellation ID was reused")
		}
		return nil
	}
	report := m.Status(ctx, name)
	if report.State != "running" || report.AccessStatus != "verified" || !report.SupportsMatchFence {
		return errors.New("MatchZy cancellation is unavailable")
	}
	return access.Write(path, access.MatchCancelRequest{ID: command.ID, SetupID: command.SetupID})
}
