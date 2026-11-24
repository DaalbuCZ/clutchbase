package manager

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func (m *Manager) RequestLiveMode(ctx context.Context, name string, command protocol.ModeCommand) error {
	if !protocol.ValidCommandID(command.ID) || (command.Mode != "match" && command.Mode != "practice") {
		return errors.New("invalid live mode request")
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	report := m.Status(ctx, name)
	if report.State != "running" || report.AccessStatus != "verified" || !report.CanLiveMode {
		return errors.New("live mode control is unavailable")
	}
	if report.ActivePreset == command.Mode {
		return errors.New("server already reports the requested mode")
	}
	path := filepath.Join(m.accessDir(name), "mode-request.json")
	var previous access.ModeCommand
	if access.Read(path, &previous) == nil && previous.ID == command.ID {
		if previous.Mode != command.Mode {
			return errors.New("live mode request ID was reused")
		}
		return nil
	}
	command.RequestedAt = time.Now().UnixMilli()
	return access.Write(path, command)
}
