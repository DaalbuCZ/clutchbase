package protocol

import (
	"testing"
	"time"
)

func TestPresetPolicy(t *testing.T) {
	now := time.Now()
	desired := "practice"
	old := "match"
	h := Host{Map: "de_mirage", MachineID: "machine", CanPreset: true, LastSeen: &now, DesiredState: "stopped", ObservedState: "stopped", Preset: &desired, AppliedPreset: &old}
	h.Project(now)
	if !h.Actions.ChangePreset || h.Actions.Start || h.PresetApplied {
		t.Fatal("unapplied preset allowed start")
	}
	h.AppliedPreset = &desired
	h.Project(now)
	if !h.Actions.Start || !h.PresetApplied {
		t.Fatal("matching configuration not acknowledged")
	}
	h.ObservedState = "running"
	h.DesiredState = "running"
	h.ActivePreset = "match"
	h.Project(now)
	if h.Actions.ChangePreset || h.PresetStatus != "mismatch" {
		t.Fatal("live mode mismatch not projected")
	}
	h.ActivePreset = "practice"
	h.Project(now)
	if h.PresetStatus != "verified" {
		t.Fatal("matching runtime mode not projected")
	}
	h.CanPreset = false
	h.DesiredState = "stopped"
	h.ObservedState = "stopped"
	h.Project(now)
	if h.Actions.Start || h.Actions.ChangePreset {
		t.Fatal("old agent can act on managed preset")
	}
	h.LastSeen = nil
	h.Project(now)
	if h.ActivePreset != "" || h.PresetStatus != "unknown" {
		t.Fatal("stale runtime mode exposed")
	}
	for _, bad := range []string{"casual", "practice;quit", "../practice"} {
		if ValidPreset(bad) {
			t.Fatal("invalid preset accepted")
		}
	}
}
