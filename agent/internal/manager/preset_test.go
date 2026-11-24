package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func presetFixture(t *testing.T, m *Manager) {
	t.Helper()
	m.PresetPlugin = t.TempDir()
	for name, value := range map[string]string{"ClutchbaseAccess.dll": "preset reporter fixture", "presets.version": "1\n", "mode-control.version": "1\n"} {
		if err := os.WriteFile(filepath.Join(m.PresetPlugin, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPresetStoppedOnlyRecoveryAndPasswordPreservation(t *testing.T) {
	ctx := context.Background()
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	presetFixture(t, m)
	if err := m.SetPreset(ctx, in.Name, "practice"); err == nil {
		t.Fatal("configured uninstalled game")
	}
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(ctx, in.Name, "practice"); err == nil {
		t.Fatal("configured without required plugins")
	}
	addPluginFixture(t, m, in.Name)
	if err := m.SetPreset(ctx, in.Name, "practice"); err != nil {
		t.Fatal(err)
	}
	configured, err := m.Load(in.Name)
	if err != nil || configured.Preset != "practice" || configured.Password != in.Password {
		t.Fatal("private configuration or password lost", err)
	}
	override, _ := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if !strings.Contains(string(override), "matchzy_autostart_mode 2\n") || !strings.Contains(string(override), "whitelist_enabled_default false") {
		t.Fatal("wrong practice override")
	}
	if err := m.checkPresetReporter(in.Name); err != nil {
		t.Fatal(err)
	}
	if err := m.SyncPreset(ctx, in.Name, "practice"); err != nil {
		t.Fatal("idempotent sync failed", err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CB_TEST_STATE"), unit(in.Name)), []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(ctx, in.Name, "match"); err == nil {
		t.Fatal("changed preset on active game")
	}
	if err := m.SyncPreset(ctx, in.Name, "practice"); err != nil {
		t.Fatal("matching active preset was rewritten", err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CB_TEST_STATE"), unit(in.Name)), []byte("inactive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(ctx, in.Name, "match"); err != nil {
		t.Fatal(err)
	}
	override, _ = os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if !strings.Contains(string(override), "matchzy_autostart_mode 1\n") {
		t.Fatal("wrong match override")
	}
	// A persisted clear with an interrupted override write must not be acknowledged.
	configured, _ = m.Load(in.Name)
	configured.Preset = ""
	data, _ := json.Marshal(configured)
	dir, _ := m.Dir(in.Name)
	if err := atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		t.Fatal(err)
	}
	if err := m.SyncPreset(ctx, in.Name, ""); err != nil {
		t.Fatal(err)
	}
	override, _ = os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if strings.Contains(string(override), "matchzy_autostart_mode") {
		t.Fatal("interrupted default clear not repaired")
	}
	if err := m.SetPreset(ctx, in.Name, "practice;quit"); err == nil {
		t.Fatal("arbitrary preset command accepted")
	}
}

func TestPresetPreservesCustomOverridesAndAgentLease(t *testing.T) {
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	ctx := context.Background()
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	addPluginFixture(t, m, in.Name)
	presetFixture(t, m)
	custom := []byte("// operator config\n")
	if err := os.MkdirAll(m.whitelistDir(in.Name, ""), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.whitelistDir(in.Name, "config_override.cfg"), custom, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.SetPreset(ctx, in.Name, "match"); err == nil {
		t.Fatal("overwrote custom configuration")
	}
	actual, _ := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if string(actual) != string(custom) {
		t.Fatal("custom configuration changed")
	}
	release, err := m.AcquireAgent(in.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	other := *m
	other.agentName = ""
	if err := other.SetPreset(ctx, in.Name, "practice"); err == nil {
		t.Fatal("bypassed active controller lease")
	}
}
