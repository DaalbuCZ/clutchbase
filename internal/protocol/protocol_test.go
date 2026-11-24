package protocol

import (
	"strings"
	"testing"
	"time"
)

func TestLifecyclePolicy(t *testing.T) {
	now := time.Now()
	old := now.Add(-21 * time.Second)
	for _, tc := range []struct {
		name, desired, observed, message, status string
		seen                                     *time.Time
		actions                                  Actions
	}{
		{"unlinked", "stopped", "unknown", "", "offline", nil, Actions{}},
		{"stale", "running", "running", "", "offline", &old, Actions{}},
		{"stopped", "stopped", "stopped", "", "stopped", &now, Actions{Start: true, ChangeMap: true}},
		{"uninstalled", "stopped", "uninstalled", "", "uninstalled", &now, Actions{Start: true, ChangeMap: true}},
		{"installation", "running", "installing", "", "installing", &now, Actions{Stop: true}},
		{"cancelling installation", "stopped", "installing", "", "stopping", &now, Actions{}},
		{"start requested", "running", "stopped", "", "starting", &now, Actions{Stop: true}},
		{"running", "running", "running", "", "running", &now, Actions{Stop: true}},
		{"stop requested", "stopped", "running", "", "stopping", &now, Actions{}},
		{"failed start", "running", "error", "failed", "error", &now, Actions{Stop: true}},
		{"recoverable error", "stopped", "error", "failed", "error", &now, Actions{Start: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Host{Map: "de_mirage", DesiredState: tc.desired, ObservedState: tc.observed, Error: tc.message, LastSeen: tc.seen}
			h.Project(now)
			if h.Status != tc.status || h.Actions != tc.actions {
				t.Fatalf("got %s %+v; want %s %+v", h.Status, h.Actions, tc.status, tc.actions)
			}
		})
	}
}

func TestGameUpdateRequiresCurrentStoppedManagedHost(t *testing.T) {
	now := time.Now()
	h := Host{Map: "de_mirage", CanUpdateGame: true, CanReporterUpdate: true, CanRemove: true, CanWhitelist: true, CanPreset: true, ReporterSourceHash: strings.Repeat("a", 64), MachineID: "machine", Driver: "managed", DesiredState: "stopped", ObservedState: "stopped", LastSeen: &now}
	h.Project(now)
	if !h.Actions.UpdateGame {
		t.Fatal("current stopped managed server cannot be updated")
	}
	h.GameUpdateStatus = "pending"
	h.Project(now)
	if h.Actions.UpdateGame || h.Actions.UpdateReporter || h.Actions.Start || h.Actions.ChangeMap || h.Actions.ChangeWhitelist || h.Actions.ChangePreset || h.Actions.Remove {
		t.Fatal("pending game update exposed conflicting actions")
	}
	h.GameUpdateStatus = ""
	h.ReporterUpdateStatus = "pending"
	h.Project(now)
	if h.Actions.UpdateGame || h.Actions.UpdateReporter || h.Actions.Start || h.Actions.ChangeMap || h.Actions.ChangeWhitelist || h.Actions.ChangePreset || h.Actions.Remove {
		t.Fatal("pending reporter update exposed conflicting actions")
	}
	h.ReporterUpdateStatus = ""
	h.Driver = "simulated"
	h.Project(now)
	if h.Actions.UpdateGame {
		t.Fatal("simulation exposed a real game update")
	}
}

func TestLiveModeCommandPolicyAndIDFormat(t *testing.T) {
	now := time.Now()
	base := Host{
		CanLiveMode: true, ActivePreset: "match", ModeCommandStatus: "complete",
		DesiredState: "running", ObservedState: "running", LastSeen: &now,
	}
	base.Project(now)
	if !base.Actions.SwitchMode {
		t.Fatal("verified running match server cannot switch modes")
	}
	for _, mutate := range []func(*Host){
		func(h *Host) { h.CanLiveMode = false },
		func(h *Host) { h.ModeCommandStatus = "pending" },
		func(h *Host) { h.ActivePreset = "" },
		func(h *Host) { h.ObservedState = "starting" },
		func(h *Host) { h.DesiredState = "stopped" },
	} {
		h := base
		mutate(&h)
		h.Project(now)
		if h.Actions.SwitchMode {
			t.Fatalf("unsafe live-mode state enabled switching: %+v", h)
		}
	}
	if !ValidCommandID("ABCDEFGHIJKLMNOPQRSTUVWXYZ") || ValidCommandID("abcdefghijklmnopqrstuvwxyz") || ValidCommandID("ABC") {
		t.Fatal("command IDs do not follow crypto/rand.Text's base32 alphabet")
	}
}

func TestStaleAccessEvidenceIsNotProjectedAsVerified(t *testing.T) {
	old := time.Now().Add(-21 * time.Second)
	h := Host{Map: "de_mirage", AccessStatus: "verified", LastSeen: &old}
	h.Project(time.Now())
	if h.AccessStatus != "unknown" {
		t.Fatal("offline server claims current verification")
	}
}

func TestRemovalPolicy(t *testing.T) {
	now := time.Now()
	base := Host{MachineID: "machine", CanRemove: true, DesiredState: "stopped", ObservedState: "stopped", LastSeen: &now}
	base.Project(now)
	if !base.Actions.Remove {
		t.Fatal("stopped fleet instance cannot be removed")
	}
	for _, mutate := range []func(*Host){
		func(h *Host) { h.CanRemove = false },
		func(h *Host) { h.MachineID = "" },
		func(h *Host) { h.LastSeen = nil },
		func(h *Host) { h.DesiredState = "running" },
		func(h *Host) { h.ObservedState = "running" },
		func(h *Host) { h.ObservedState = "installing" },
	} {
		h := base
		mutate(&h)
		h.Project(now)
		if h.Actions.Remove {
			t.Fatal("offered unsafe removal")
		}
	}
	base.Removing = true
	base.LastSeen = nil
	base.Project(now)
	if base.Status != "removing" || base.Actions != (Actions{}) {
		t.Fatal("pending removal exposed lifecycle actions")
	}
}

func TestGameBuildStatusRequiresFreshObservation(t *testing.T) {
	now := time.Now()
	h := Host{Map: "de_mirage", Driver: "managed", ObservedState: "running", GameBuildInstalled: "123", GameBuildLatest: "456", GameBuildCheckedAt: &now, LastSeen: &now}
	h.Project(now)
	if h.GameBuildStatus != "outdated" {
		t.Fatalf("status = %q", h.GameBuildStatus)
	}
	h.GameBuildLatest = "123"
	h.Project(now)
	if h.GameBuildStatus != "current" {
		t.Fatalf("status = %q", h.GameBuildStatus)
	}
	h.Project(now.Add(3 * time.Hour))
	if h.GameBuildStatus != "unknown" {
		t.Fatalf("stale status = %q", h.GameBuildStatus)
	}
}

func TestMachineResourcesValidation(t *testing.T) {
	cpu := 50.0
	valid := MachineResources{CPUPercent: &cpu, MemoryTotalBytes: 1000, MemoryAvailableBytes: 200}
	if !valid.Valid() {
		t.Fatal("valid resources rejected")
	}
	cpu = -1
	if valid.Valid() {
		t.Fatal("negative CPU accepted")
	}
	cpu = 101
	if valid.Valid() {
		t.Fatal("CPU over 100 accepted")
	}
	valid.CPUPercent = nil
	if !valid.Valid() {
		t.Fatal("unknown CPU rejected")
	}
	valid.MemoryTotalBytes = 0
	if valid.Valid() {
		t.Fatal("zero memory capacity accepted")
	}
}

func TestRestartCommandValidation(t *testing.T) {
	id := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for _, phase := range []string{"stop", "start", "cancel"} {
		if !(&RestartCommand{ID: id, Phase: phase}).Valid() {
			t.Fatalf("valid %s rejected", phase)
		}
	}
	for _, command := range []*RestartCommand{nil, {ID: "bad", Phase: "stop"}, {ID: id, Phase: "restart"}, {ID: id, Phase: ""}} {
		if command.Valid() {
			t.Fatalf("invalid restart command accepted: %+v", command)
		}
	}
}

func TestRestartActionsFollowCanonicalPhase(t *testing.T) {
	now := time.Now()
	h := Host{Map: "de_mirage", MachineID: "machine", Driver: "managed", DesiredState: "running", ObservedState: "running", LastSeen: &now, CanRestart: true}
	h.Project(now)
	if !h.Actions.Restart {
		t.Fatal("healthy server did not offer restart")
	}
	h.RestartPhase = "stopping"
	h.DesiredState = "stopped"
	h.ObservedState = "stopped"
	h.Project(now)
	if h.Actions.Restart || h.Actions.Start || !h.Actions.CancelRestart || h.Actions.UpdateGame {
		t.Fatalf("unexpected pending actions: %+v", h.Actions)
	}
	h.RestartPhase = "cancelling"
	h.Project(now)
	if h.Actions.CancelRestart || h.Actions.Start {
		t.Fatalf("cancelling exposed an action: %+v", h.Actions)
	}
	h.LastSeen = nil
	h.Project(now)
	if h.Actions.CancelRestart || h.Actions.Restart {
		t.Fatal("offline restart action")
	}
}

func TestStopAndUpdateRequiresIdleConnectedManagedServer(t *testing.T) {
	now := time.Now()
	base := Host{CanUpdateGame: true, MachineID: "machine", Driver: "managed", DesiredState: "running", ObservedState: "running", LastSeen: &now, ActivePreset: "match", AccessStatus: "verified", MatchPhase: "idle"}
	base.Project(now)
	if !base.Actions.StopAndUpdate {
		t.Fatalf("idle running server cannot be updated: %+v", base)
	}
	for _, tc := range []struct {
		name   string
		change func(*Host)
	}{
		{"offline", func(h *Host) { h.LastSeen = nil }},
		{"old agent", func(h *Host) { h.CanUpdateGame = false }},
		{"stopped", func(h *Host) { h.DesiredState = "stopped" }},
		{"error", func(h *Host) { h.Error = "error" }},
		{"setup", func(h *Host) { h.MatchPhase = "setup" }},
		{"unknown match state", func(h *Host) { h.MatchPhase = "unknown" }},
		{"live", func(h *Host) { h.MatchPhase = "live" }},
		{"paused", func(h *Host) { h.MatchPhase = "paused" }},
		{"game update", func(h *Host) { h.GameUpdateStatus = "pending" }},
		{"plugin update", func(h *Host) { h.ReporterUpdateStatus = "pending" }},
		{"restart", func(h *Host) { h.RestartPhase = "stopping" }},
		{"mode change", func(h *Host) { h.ModeCommandStatus = "pending" }},
		{"match control", func(h *Host) { h.MatchControlStatus = "pending" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := base
			tc.change(&h)
			h.Project(now)
			if h.Actions.StopAndUpdate {
				t.Fatalf("unsafe action: %+v", h)
			}
		})
	}
}

func TestReporterInventoryUsesFreshAgentEvidence(t *testing.T) {
	now := time.Now()
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		name, source, installed, driver, want string
		capable                               bool
		age                                   time.Duration
	}{
		{"current", a, a, "managed", "current", true, 0},
		{"different", a, b, "managed", "different", true, 0},
		{"missing or invalid", a, "", "managed", "unavailable", true, 0},
		{"no staged bundle", "", a, "managed", "unknown", true, 0},
		{"offline", a, a, "managed", "unknown", true, 21 * time.Second},
		{"old agent", a, a, "managed", "unknown", false, 0},
		{"simulation", a, a, "simulated", "unknown", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := now.Add(-tc.age)
			h := Host{Map: "de_mirage", LastSeen: &seen, Driver: tc.driver, CanReporterUpdate: tc.capable, ReporterSourceHash: tc.source, ReporterInstalledHash: tc.installed}
			h.Project(now)
			if h.ReporterInventory.Status != tc.want {
				t.Fatalf("inventory status = %q, want %q", h.ReporterInventory.Status, tc.want)
			}
			if tc.capable && tc.driver == "managed" && (h.ReporterInventory.CheckedAt == nil || h.ReporterInventory.InstalledHash != tc.installed) {
				t.Fatalf("last report not retained: %+v", h.ReporterInventory)
			}
		})
	}
}
