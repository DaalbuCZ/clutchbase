package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Retire preserves the installation but removes its inactive managed unit.
// Repeating it after a lost API acknowledgement is safe.
func (m *Manager) Retire(ctx context.Context, name string) error {
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	dir, _ := m.Dir(name)
	unlockCreate, err := lock(filepath.Join(m.Root, "create.lock"))
	if err != nil {
		return err
	}
	defer unlockCreate()
	info, err := os.Lstat(dir)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if exists {
		if !info.IsDir() {
			return errors.New("instance path is not a private directory")
		}
		unlock, err := lock(filepath.Join(dir, "operation.lock"))
		if err != nil {
			return err
		}
		defer unlock()
		if _, err := m.Load(name); err != nil {
			return errors.New("cannot verify the local instance configuration")
		}
	}
	if err := m.checkUnit(name); err != nil {
		return err
	}
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state == "error" {
		if err := systemctl(ctx, "stop", unit(name)); err != nil {
			return err
		}
		if err := systemctl(ctx, "reset-failed", unit(name)); err != nil {
			return err
		}
		state, err = serviceState(ctx, unit(name))
		if err != nil {
			return err
		}
	}
	if state != "stopped" {
		return errors.New("local service is not stopped; stop it before retrying removal")
	}
	if err := os.Remove(filepath.Join(m.UnitDir, unit(name))); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	archive := filepath.Join(m.Root, "retired", name)
	if _, err := os.Lstat(archive); !errors.Is(err, os.ErrNotExist) {
		return errors.New("retained instance directory already exists; preserve it before retrying removal")
	}
	if err := os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		return err
	}
	return os.Rename(dir, archive)
}
