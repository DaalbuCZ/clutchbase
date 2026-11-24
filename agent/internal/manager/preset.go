package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"clutchbase/internal/protocol"
)

func (m *Manager) SetPreset(ctx context.Context, name, preset string) error {
	if !protocol.ValidPreset(preset) {
		return errors.New("use match, practice or an empty default preset")
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	in, err := m.Load(name)
	if err != nil {
		return err
	}
	dir, _ := m.Dir(name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state != "stopped" {
		return errors.New("stop the instance before changing its preset")
	}
	if preset != "" && !m.installed(name) {
		return errors.New("install the instance before selecting a preset")
	}
	in.Preset = preset
	if err := m.checkWhitelist(in); err != nil {
		return err
	}
	if preset != "" {
		if err := m.installPresetReporter(name); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		return err
	}
	// Persist intent first; a partial override write is repaired before launch.
	return m.writeWhitelist(in)
}

func (m *Manager) SyncPreset(ctx context.Context, name, preset string) error {
	if !protocol.ValidPreset(preset) {
		return errors.New("unsupported preset")
	}
	in, err := m.Load(name)
	if err != nil {
		return err
	}
	if in.Preset == preset {
		if preset == "" {
			data, err := os.ReadFile(m.whitelistDir(name, "config_override.cfg"))
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			if !strings.HasPrefix(string(data), whitelistMarker) || string(data) == whitelistOverride(in) {
				return nil
			}
			return m.SetPreset(ctx, name, preset)
		}
		if err := m.checkWhitelist(in); err != nil {
			return err
		}
		if err := m.checkPresetReporter(name); err != nil {
			return err
		}
		data, err := os.ReadFile(m.whitelistDir(name, "config_override.cfg"))
		if err == nil && string(data) == whitelistOverride(in) {
			return nil
		}
	}
	return m.SetPreset(ctx, name, preset)
}

func (m *Manager) presetPluginDir(name string) string {
	dir, _ := m.Dir(name)
	return filepath.Join(dir, "server/game/csgo/addons/counterstrikesharp/plugins/ClutchbaseAccess")
}

func (m *Manager) checkPresetReporter(name string) error {
	for _, marker := range []string{"presets.version", "mode-control.version"} {
		data, err := os.ReadFile(filepath.Join(m.presetPluginDir(name), marker))
		if err != nil || strings.TrimSpace(string(data)) != "1" {
			return errors.New("update the Clutchbase Access plugin before using presets; select the preset again while stopped")
		}
	}
	return nil
}

func (m *Manager) installPresetReporter(name string) error {
	data, versions, _, err := readReporterBundle(m.PresetPlugin)
	if err != nil {
		return err
	}
	return m.writeReporterBundle(name, data, versions)
}

func (m *Manager) writeReporterBundle(name string, data []byte, versions map[string][]byte) error {
	dir := m.presetPluginDir(name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "ClutchbaseAccess.dll"), data); err != nil {
		return err
	}
	for marker, version := range versions {
		if err := atomicWrite(filepath.Join(dir, marker), version); err != nil {
			return err
		}
	}
	return nil
}
