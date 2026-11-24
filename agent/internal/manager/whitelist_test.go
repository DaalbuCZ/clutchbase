package manager

import (
	"clutchbase/agent/internal/access"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func addPluginFixture(t *testing.T, m *Manager, name string) {
	t.Helper()
	dir, _ := m.Dir(name)
	for _, file := range []string{
		"addons/metamod/bin/linuxsteamrt64/libserver.so",
		"addons/metamod/bin/linuxsteamrt64/metamod.2.cs2.so",
		"addons/metamod/counterstrikesharp.vdf",
		"addons/counterstrikesharp/bin/linuxsteamrt64/counterstrikesharp.so",
		"addons/counterstrikesharp/plugins/MatchZy/MatchZy.dll",
		"addons/counterstrikesharp/plugins/ClutchbaseAccess/ClutchbaseAccess.dll",
		"gameinfo.gi",
	} {
		path := filepath.Join(dir, "server", "game", "csgo", filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("Game    csgo/addons/metamod\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWhitelistValidation(t *testing.T) {
	for _, ids := range [][]string{
		{"76561197960265728"}, {"76561202255233024"}, {"+76561198154367261"},
		{"76561198154367261;quit"}, {"76561198154367261", "76561198154367261"},
		{"76561198154367261\nquit"}, {"STEAM_0:1:1"}, make([]string, 257),
	} {
		if err := validateWhitelist(ids); err == nil {
			t.Fatalf("accepted invalid IDs: %q", ids)
		}
	}
	if err := validateWhitelist([]string{"76561197960265729", "76561202255233023"}); err != nil {
		t.Fatal(err)
	}
}

func TestWhitelistLifecycleAndRecovery(t *testing.T) {
	ctx := context.Background()
	m := fixture(t)
	m.Guard = filepath.Join(t.TempDir(), "guard")
	if err := os.WriteFile(m.Guard, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	in := create(t, m, "friends", 27015)
	ids := []string{"76561198154367261"}
	if err := m.SetWhitelist(ctx, in.Name, ids); err == nil {
		t.Fatal("configured an uninstalled instance")
	}
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	if err := m.SetWhitelist(ctx, in.Name, ids); err == nil {
		t.Fatal("configured a whitelist without plugins")
	}
	addPluginFixture(t, m, in.Name)
	if err := m.SetWhitelist(ctx, in.Name, ids); err != nil {
		t.Fatal(err)
	}
	if err := m.SyncWhitelist(ctx, in.Name, ids); err != nil {
		t.Fatal(err)
	}
	presetFixture(t, m)
	if err := m.SetPreset(ctx, in.Name, "practice"); err != nil {
		t.Fatal(err)
	}
	loaded, err := m.Load(in.Name)
	if err != nil || len(loaded.Whitelist) != 1 || loaded.Password != in.Password {
		t.Fatalf("lost private configuration: %v", err)
	}
	for _, file := range []string{"whitelist.cfg", "config_override.cfg"} {
		info, err := os.Stat(m.whitelistDir(in.Name, file))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("whitelist file not private: %s, %v", file, err)
		}
	}
	// The host rewrites a missing roster before launch, preserving the password.
	if err := os.Remove(m.whitelistDir(in.Name, "whitelist.cfg")); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, in.Name, "de_nuke"); err != nil {
		t.Fatal(err)
	}
	roster, _ := os.ReadFile(m.whitelistDir(in.Name, "whitelist.cfg"))
	if string(roster) != ids[0]+"\n" {
		t.Fatal("roster not restored")
	}
	dir, _ := m.Dir(in.Name)
	cfg, _ := os.ReadFile(filepath.Join(dir, "server/game/csgo/cfg/server.cfg"))
	if !strings.Contains(string(cfg), "sv_password \""+in.Password+"\"") {
		t.Fatal("whitelist removed password protection")
	}
	if report := m.Status(ctx, in.Name); report.AccessStatus != "pending" || report.State != "starting" {
		t.Fatalf("premature verification: %+v", report)
	}
	var request access.Request
	if err := access.Read(filepath.Join(m.accessDir(in.Name), "request.json"), &request); err != nil {
		t.Fatal(err)
	}
	request.ProcessGroup = syscall.Getpgrp()
	if err := access.Write(filepath.Join(m.accessDir(in.Name), "request.json"), request); err != nil {
		t.Fatal(err)
	}
	if err := access.Write(filepath.Join(m.accessDir(in.Name), "evidence.json"), access.Evidence{Request: request, ProcessID: os.Getpid(), ReportedAt: time.Now().UnixMilli(), Status: "verified", Mode: "match", CanLoadMatch: true, SupportsManualReady: true}); err != nil {
		t.Fatal(err)
	}
	serveA2S(t, in.Port, 0, 12)
	if report := m.Status(ctx, in.Name); report.AccessStatus != "verified" || report.State != "running" || report.ActivePreset != "match" || !report.SupportsManualReady {
		t.Fatalf("runtime evidence not projected: %+v", report)
	}
	if err := access.Write(filepath.Join(m.accessDir(in.Name), "evidence.json"), access.Evidence{Request: request, ProcessID: os.Getpid(), ReportedAt: time.Now().UnixMilli(), Status: "verified", Mode: "practice", SupportsManualReady: true}); err != nil {
		t.Fatal(err)
	}
	if report := m.Status(ctx, in.Name); report.SupportsManualReady {
		t.Fatalf("manual-ready capability escaped unavailable MatchZy runtime: %+v", report)
	}
	if err := access.Write(filepath.Join(m.accessDir(in.Name), "failure.json"), "failed"); err != nil {
		t.Fatal(err)
	}
	if report := m.Status(ctx, in.Name); report.AccessStatus != "blocked" || report.State != "error" {
		t.Fatalf("watchdog failure not projected: %+v", report)
	}
	if err := m.Start(ctx, in.Name, "de_nuke"); err == nil {
		t.Fatal("watchdog failure did not block automatic restart")
	}
	requestBefore, _ := os.ReadFile(filepath.Join(m.accessDir(in.Name), "request.json"))
	if err := m.SyncWhitelist(ctx, in.Name, ids); err != nil {
		t.Fatal("matching running roster rejected", err)
	}
	requestAfter, _ := os.ReadFile(filepath.Join(m.accessDir(in.Name), "request.json"))
	if string(requestBefore) != string(requestAfter) {
		t.Fatal("sync changed running launch nonce")
	}
	if err := m.SyncWhitelist(ctx, in.Name, []string{"76561197960265729"}); err == nil {
		t.Fatal("changed a live roster")
	}
	if err := m.SetWhitelist(ctx, in.Name, nil); err == nil {
		t.Fatal("disabled whitelist on a running instance")
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("CB_TEST_STATE"), unit(in.Name)), []byte("failed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(ctx, in.Name); err != nil {
		t.Fatal(err)
	}
	if access.Failure(m.accessDir(in.Name)) {
		t.Fatal("Stop did not reset watchdog latch")
	}
	if err := m.SetPreset(ctx, in.Name, ""); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(dir, "server/game/csgo/addons/counterstrikesharp/plugins/MatchZy/MatchZy.dll")
	if err := os.Remove(plugin); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, in.Name, "de_nuke"); err == nil {
		t.Fatal("started without the configured whitelist plugin")
	}
	addPluginFixture(t, m, in.Name)
	// A game update that removes Metamod's search path prevents another start.
	if err := os.WriteFile(filepath.Join(dir, "server/game/csgo/gameinfo.gi"), []byte("// Game csgo/addons/metamod\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, in.Name, "de_nuke"); err == nil {
		t.Fatal("started with a missing Metamod search path")
	}
	if err := os.Remove(filepath.Join(dir, "installed")); err != nil {
		t.Fatal(err)
	}
	if err := m.SetWhitelist(ctx, in.Name, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	// A partial disable is repaired on the next start from persisted intent.
	if err := os.WriteFile(m.whitelistDir(in.Name, "config_override.cfg"), []byte(whitelistMarker+"matchzy_whitelist_enabled_default true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, in.Name, "de_nuke"); err != nil {
		t.Fatal(err)
	}
	override, _ := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if !strings.Contains(string(override), "matchzy_whitelist_enabled_default false") {
		t.Fatal("disable not repaired")
	}
}

func TestWhitelistPreservesOwnershipAndCredentials(t *testing.T) {
	ctx := context.Background()
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	addPluginFixture(t, m, in.Name)
	ids := []string{"76561198154367261"}
	release, err := m.AcquireAgent(in.Name)
	if err != nil {
		t.Fatal(err)
	}
	other := *m
	other.agentName = ""
	if err := other.SetWhitelist(ctx, in.Name, ids); err == nil {
		t.Fatal("CLI bypassed the agent lease")
	}
	release()
	m.agentName = ""
	if err := os.MkdirAll(m.whitelistDir(in.Name, ""), 0700); err != nil {
		t.Fatal(err)
	}
	custom := []byte("// Operator settings\nmatchzy_autostart_mode 2\n")
	if err := os.WriteFile(m.whitelistDir(in.Name, "config_override.cfg"), custom, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.SyncWhitelist(ctx, in.Name, []string{}); err == nil {
		t.Fatal("acknowledged a disabled dashboard roster over a custom override")
	}
	if err := m.SetWhitelist(ctx, in.Name, ids); err == nil {
		t.Fatal("overwrote a custom override")
	}
	actual, _ := os.ReadFile(m.whitelistDir(in.Name, "config_override.cfg"))
	if string(actual) != string(custom) {
		t.Fatal("custom configuration modified")
	}
	if err := os.Remove(m.whitelistDir(in.Name, "config_override.cfg")); err != nil {
		t.Fatal(err)
	}
	in.Password = ""
	dir, _ := m.Dir(in.Name)
	data, _ := json.Marshal(in)
	if err := atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		t.Fatal(err)
	}
	if err := m.SetWhitelist(ctx, in.Name, ids); err == nil {
		t.Fatal("enabled whitelist without a fallback password")
	}
}
