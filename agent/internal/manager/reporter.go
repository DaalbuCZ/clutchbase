package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"clutchbase/internal/protocol"
)

func readReporterBundle(dir string) ([]byte, map[string][]byte, string, error) {
	if dir == "" {
		return nil, nil, "", errors.New("staged Clutchbase Access reporter is unavailable")
	}
	versions := map[string][]byte{}
	for _, marker := range []string{"presets.version", "mode-control.version"} {
		version, err := os.ReadFile(filepath.Join(dir, marker))
		if err != nil || strings.TrimSpace(string(version)) != "1" {
			return nil, nil, "", errors.New("staged reporter is missing required version markers")
		}
		versions[marker] = version
	}
	f, err := os.Open(filepath.Join(dir, "ClutchbaseAccess.dll"))
	if err != nil {
		return nil, nil, "", errors.New("staged Clutchbase Access reporter is unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, "", errors.New("staged reporter is not a regular file")
	}
	const maxReporterSize = 16 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxReporterSize+1))
	if err != nil || len(data) == 0 || len(data) > maxReporterSize {
		return nil, nil, "", errors.New("staged reporter has an invalid size")
	}
	hash := sha256.Sum256(data)
	return data, versions, hex.EncodeToString(hash[:]), nil
}

func (m *Manager) reporterHashes(name string) (string, string) {
	_, _, source, _ := readReporterBundle(m.PresetPlugin)
	_, _, installed, _ := readReporterBundle(m.presetPluginDir(name))
	return source, installed
}

// ReporterInfo describes files on disk, not the plugin loaded in a running game.
type ReporterInfo struct {
	StagedHash    string `json:"stagedHash,omitempty"`
	InstalledHash string `json:"installedHash,omitempty"`
	Status        string `json:"status"`
}

func (m *Manager) ReporterStatus(name string) (ReporterInfo, error) {
	if _, err := m.Load(name); err != nil {
		return ReporterInfo{}, err
	}
	staged, installed := m.reporterHashes(name)
	info := ReporterInfo{StagedHash: staged, InstalledHash: installed, Status: "unavailable"}
	switch {
	case staged == "":
	case installed == "":
		info.Status = "not_installed"
	case staged == installed:
		info.Status = "current"
	default:
		info.Status = "update_available"
	}
	return info, nil
}

func (m *Manager) UpdateReporter(ctx context.Context, name, expected string) error {
	if !protocol.ValidReporterHash(expected) {
		return errors.New("invalid staged reporter fingerprint")
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	in, err := m.Load(name)
	if err != nil {
		return errors.New("cannot read server configuration")
	}
	dir, err := m.Dir(name)
	if err != nil {
		return errors.New("invalid server")
	}
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		return errors.New("server is busy")
	}
	defer unlock()
	state, err := serviceState(ctx, unit(name))
	if err != nil || state != "stopped" {
		return errors.New("stop the server before installing the reporter")
	}
	if !m.installed(name) {
		return errors.New("install the server before updating the reporter")
	}
	if err = m.checkWhitelistFiles(in, false); err != nil {
		return errors.New("the existing plugin stack is not ready")
	}
	_, _, installed, _ := readReporterBundle(m.presetPluginDir(name))
	if installed == expected {
		return nil
	}
	data, versions, source, err := readReporterBundle(m.PresetPlugin)
	if err != nil {
		return err
	}
	if source != expected {
		return errors.New("the staged reporter changed; refresh and try again")
	}
	if err = m.writeReporterBundle(name, data, versions); err != nil {
		return errors.New("could not install the staged reporter")
	}
	_, _, installed, err = readReporterBundle(m.presetPluginDir(name))
	if err != nil || installed != expected {
		return errors.New("could not verify the installed reporter")
	}
	return nil
}
