package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"clutchbase/internal/protocol"
)

// restartFence describes one request's durable acknowledgements, not a process state.
// Only the request that was issued to the agent can advance its own phase.
type restartFence struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
}

func (f restartFence) advance(id, event string) (restartFence, error) {
	if !protocol.ValidCommandID(id) || f.ID != id {
		return f, errors.New("restart request mismatch")
	}
	switch {
	case f.Phase == "stopping" && event == "stopped":
		f.Phase = "stopped"
	case f.Phase == "stopped" && event == "start_authorized":
		f.Phase = "starting"
	case f.Phase == "starting" && event == "running":
		f.Phase = "complete"
	case (f.Phase == "stopping" || f.Phase == "stopped" || f.Phase == "starting") && event == "cancel":
		f.Phase = "cancelled"
	case (f.Phase == "stopping" || f.Phase == "starting") && event == "failed":
		f.Phase = "failed"
	default:
		return f, errors.New("invalid restart transition")
	}
	return f, nil
}

// saveRestartFence persists the acknowledged phase before it may be reported.
func saveRestartFence(dir string, f restartFence) error {
	if !protocol.ValidCommandID(f.ID) {
		return errors.New("invalid restart ID")
	}
	switch f.Phase {
	case "stopping", "stopped", "starting", "complete", "cancelled", "failed":
	default:
		return errors.New("invalid restart phase")
	}
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".restart-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(dir, "restart-state.json")); err != nil {
		return err
	}
	parent, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func readRestartFence(dir string) (restartFence, error) {
	var f restartFence
	path := filepath.Join(dir, "restart-state.json")
	info, err := os.Lstat(path)
	if err != nil {
		return f, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024 {
		return f, errors.New("invalid restart state file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err = json.Unmarshal(data, &f); err != nil || !protocol.ValidCommandID(f.ID) {
		return restartFence{}, errors.New("invalid restart state")
	}
	switch f.Phase {
	case "stopping", "stopped", "starting", "complete", "cancelled", "failed":
		return f, nil
	default:
		return restartFence{}, errors.New("invalid restart phase")
	}
}

// recordRestartFence serializes transitions for a locally configured instance.
// It records intent/acknowledgement only and never starts or stops the service.
func (m *Manager) recordRestartFence(ctx context.Context, name, id, event string) (restartFence, error) {
	var empty restartFence
	if !protocol.ValidCommandID(id) {
		return empty, errors.New("invalid restart ID")
	}
	dir, err := m.Dir(name)
	if err != nil {
		return empty, err
	}
	if _, err := m.Load(name); err != nil {
		return empty, err
	}
	unlock, err := waitLock(ctx, filepath.Join(dir, "restart-state.lock"))
	if err != nil {
		return empty, err
	}
	defer unlock()
	current, err := readRestartFence(dir)
	if errors.Is(err, os.ErrNotExist) {
		if event != "begin" && event != "cancel" {
			return empty, errors.New("restart request not recorded")
		}
		phase := "stopping"
		if event == "cancel" {
			phase = "cancelled"
		}
		current = restartFence{ID: id, Phase: phase}
	} else if err != nil {
		return empty, err
	} else if event == "cancel" && current.ID == id && current.Phase == "cancelled" {
		return current, nil
	} else if event == "cancel" && current.ID != id && (current.Phase == "complete" || current.Phase == "cancelled" || current.Phase == "failed") {
		current = restartFence{ID: id, Phase: "cancelled"}
	} else if event == "begin" {
		if current.ID == id {
			return current, nil
		}
		if current.Phase != "complete" && current.Phase != "cancelled" && current.Phase != "failed" {
			return empty, errors.New("another restart is in progress")
		}
		current = restartFence{ID: id, Phase: "stopping"}
	} else {
		current, err = current.advance(id, event)
		if err != nil {
			return empty, err
		}
	}
	if err := saveRestartFence(dir, current); err != nil {
		return empty, err
	}
	return current, nil
}

// RecordRestartFence persists one runner acknowledgement for a restart request.
func (m *Manager) RecordRestartFence(ctx context.Context, name, id, event string) (string, string, error) {
	f, err := m.recordRestartFence(ctx, name, id, event)
	return f.ID, f.Phase, err
}

// RestartFence returns the last durable request acknowledgement.
func (m *Manager) RestartFence(name string) (string, string, error) {
	dir, err := m.Dir(name)
	if err != nil {
		return "", "", err
	}
	f, err := readRestartFence(dir)
	return f.ID, f.Phase, err
}
