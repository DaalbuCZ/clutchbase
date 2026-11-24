package manager

import (
	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const whitelistMarker = "// Managed by Clutchbase whitelist.\n"

func validateWhitelist(ids []string) error { return protocol.ValidateWhitelist(ids) }

// SetWhitelist configures an already installed MatchZy stack. Passwords remain
// mandatory until real client tests and runtime plugin checks prove enforcement.
func (m *Manager) SetWhitelist(ctx context.Context, name string, ids []string) error {
	if err := validateWhitelist(ids); err != nil {
		return err
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
	if len(ids) != 0 && !m.installed(name) {
		return errors.New("install the instance before configuring its whitelist")
	}
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state != "stopped" {
		return errors.New("stop the instance before configuring its whitelist")
	}
	if len(ids) == 0 && len(in.Whitelist) == 0 {
		return m.writeWhitelist(in)
	}
	next := in
	next.Whitelist = append([]string(nil), ids...)
	if err := m.checkWhitelist(next); err != nil {
		return err
	}
	if len(ids) != 0 {
		if err := m.installGuard(name); err != nil {
			return err
		}
	}
	// Persist intent first: a failed config write must be retried before Start.
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		return err
	}
	return m.writeWhitelist(next)
}

func (m *Manager) whitelistDir(name, file string) string {
	dir, _ := m.Dir(name)
	return filepath.Join(dir, "server", "game", "csgo", "cfg", "MatchZy", file)
}

func (m *Manager) checkWhitelist(in Instance) error {
	return m.checkWhitelistFiles(in, true)
}

func (m *Manager) checkWhitelistFiles(in Instance, requireReporter bool) error {
	override, err := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(override) != 0 && !strings.HasPrefix(string(override), whitelistMarker) {
		return errors.New("MatchZy config_override.cfg belongs to another configuration; preserve it before using the whitelist command")
	}
	if len(in.Whitelist) == 0 && in.Preset == "" {
		return nil
	}
	if err := validateWhitelist(in.Whitelist); err != nil {
		return err
	}
	if len(in.Whitelist) > 0 && in.Password == "" {
		return errors.New("keep a join password until whitelist enforcement is verified with real clients")
	}
	return m.checkMatchZy(in.Name, requireReporter)
}

func (m *Manager) checkMatchZy(name string, requireReporter bool) error {
	dir, _ := m.Dir(name)
	csgo := filepath.Join(dir, "server", "game", "csgo")
	for _, file := range []string{
		"addons/metamod/bin/linuxsteamrt64/libserver.so",
		"addons/metamod/bin/linuxsteamrt64/metamod.2.cs2.so",
		"addons/metamod/counterstrikesharp.vdf",
		"addons/counterstrikesharp/bin/linuxsteamrt64/counterstrikesharp.so",
		"addons/counterstrikesharp/plugins/MatchZy/MatchZy.dll",
	} {
		info, err := os.Stat(filepath.Join(csgo, filepath.FromSlash(file)))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("install Metamod, CounterStrikeSharp and MatchZy before configuring or starting a whitelisted instance; see docs/ACCESS.md")
		}
	}
	if requireReporter {
		info, err := os.Stat(filepath.Join(csgo, "addons/counterstrikesharp/plugins/ClutchbaseAccess/ClutchbaseAccess.dll"))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("install Metamod, CounterStrikeSharp and MatchZy before configuring or starting a whitelisted instance; see docs/ACCESS.md")
		}
	}
	data, err := os.ReadFile(filepath.Join(csgo, "gameinfo.gi"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "Game" && fields[1] == "csgo/addons/metamod" {
				return nil
			}
		}
	}
	return errors.New("Metamod search path missing from gameinfo.gi; see docs/ACCESS.md")
}

func (m *Manager) writeWhitelist(in Instance) error {
	if len(in.Whitelist) == 0 {
		if in.Preset != "" {
			if err := m.checkWhitelist(in); err != nil {
				return err
			}
			if err := os.MkdirAll(m.whitelistDir(in.Name, ""), 0700); err != nil {
				return err
			}
			if err := atomicWrite(m.whitelistDir(in.Name, "config_override.cfg"), []byte(whitelistOverride(in))); err != nil {
				return err
			}
			return atomicWrite(m.whitelistDir(in.Name, "whitelist.cfg"), nil)
		}
		override, err := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(override), whitelistMarker) {
			return nil
		}
		// Rewrite on every start, including after an interrupted disable command.
		if err := atomicWrite(m.whitelistDir(in.Name, "config_override.cfg"), []byte(whitelistOverride(in))); err != nil {
			return err
		}
		return atomicWrite(m.whitelistDir(in.Name, "whitelist.cfg"), nil)
	}
	if err := m.checkWhitelist(in); err != nil {
		return err
	}
	if err := os.MkdirAll(m.whitelistDir(in.Name, ""), 0700); err != nil {
		return err
	}
	if err := atomicWrite(m.whitelistDir(in.Name, "whitelist.cfg"), []byte(strings.Join(in.Whitelist, "\n")+"\n")); err != nil {
		return err
	}
	if err := atomicWrite(m.whitelistDir(in.Name, "config_override.cfg"), []byte(whitelistOverride(in))); err != nil {
		return err
	}
	if err := os.MkdirAll(m.accessDir(in.Name), 0700); err != nil {
		return err
	}
	return access.Write(filepath.Join(m.accessDir(in.Name), "request.json"), access.Request{Nonce: rand.Text(), RosterHash: access.Hash([]byte(strings.Join(in.Whitelist, "\n") + "\n"))})
}

func (m *Manager) accessDir(name string) string {
	dir, _ := m.Dir(name)
	return filepath.Join(dir, "server", "game", "csgo", "cfg", "ClutchbaseAccess")
}

func (m *Manager) installGuard(name string) error {
	info, err := os.Stat(m.Guard)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return errors.New("build clutchbase-guard beside the host and agent binaries before configuring a whitelist")
	}
	data, err := os.ReadFile(m.Guard)
	if err != nil {
		return err
	}
	dir, _ := m.Dir(name)
	path := filepath.Join(dir, "guard")
	if err := atomicWrite(path, data); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

// SyncWhitelist preserves a running launch nonce when the roster already matches.
// Interrupted writes are repaired through the stopped-only mutation path.
func (m *Manager) SyncWhitelist(ctx context.Context, name string, ids []string) error {
	if err := validateWhitelist(ids); err != nil {
		return err
	}
	in, err := m.Load(name)
	if err != nil {
		return err
	}
	if slices.Equal(in.Whitelist, ids) {
		override, readErr := os.ReadFile(m.whitelistDir(name, "config_override.cfg"))
		if readErr == nil && len(override) != 0 && !strings.HasPrefix(string(override), whitelistMarker) {
			return errors.New("MatchZy config_override.cfg belongs to another configuration; preserve it before saving a dashboard roster")
		}
		if len(ids) == 0 && (errors.Is(readErr, os.ErrNotExist) || (readErr == nil && (len(override) == 0 || string(override) == whitelistOverride(in)))) {
			return nil
		}
		roster, rosterErr := os.ReadFile(m.whitelistDir(name, "whitelist.cfg"))
		if len(ids) > 0 && readErr == nil && rosterErr == nil && string(roster) == strings.Join(ids, "\n")+"\n" && string(override) == whitelistOverride(in) {
			return m.checkWhitelist(in)
		}
	}
	return m.SetWhitelist(ctx, name, ids)
}

func whitelistOverride(in Instance) string {
	config := whitelistMarker + "matchzy_whitelist_enabled_default false\n"
	if len(in.Whitelist) > 0 {
		config = whitelistMarker + "matchzy_whitelist_enabled_default true\nmatchzy_everyone_is_admin false\n"
	}
	switch in.Preset {
	case "match":
		config += "matchzy_autostart_mode 1\n"
	case "practice":
		config += "matchzy_autostart_mode 2\n"
	}
	return config
}
