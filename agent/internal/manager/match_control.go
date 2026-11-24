package manager

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func (m *Manager) RequestMatchControl(ctx context.Context, name string, command protocol.MatchControl) error {
	if !protocol.ValidCommandID(command.ID) || !validMatchControl(command) {
		return errors.New("invalid match control request")
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(m.accessDir(name), "match-control-request.json")
	var previous access.MatchControlRequest
	if access.Read(path, &previous) == nil && previous.ID == command.ID {
		if previous.Action != command.Action || previous.SetupID != command.SetupID {
			return errors.New("match control request ID was reused")
		}
		return nil
	}
	report := m.Status(ctx, name)
	if report.State != "running" || report.AccessStatus != "verified" || report.ActivePreset != "match" || report.ActiveMatchSetupID != command.SetupID || (command.Action != "end" && !report.SupportsFencedMatchControl) || (command.Action == "pause" && !report.CanPauseMatch) || (command.Action == "resume" && !report.CanResumeMatch) {
		return errors.New("MatchZy pause control is unavailable")
	}
	return access.Write(path, access.MatchControlRequest{ID: command.ID, Action: command.Action, SetupID: command.SetupID, RequestedAt: time.Now().UnixMilli()})
}

func validMatchControl(c protocol.MatchControl) bool {
	return (c.Action == "pause" || c.Action == "resume" || c.Action == "end") && protocol.ValidCommandID(c.SetupID)
}
