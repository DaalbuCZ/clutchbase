package protocol

import (
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
		{"start requested", "running", "stopped", "", "starting", &now, Actions{Stop: true}},
		{"running", "running", "running", "", "running", &now, Actions{Stop: true}},
		{"stop requested", "stopped", "running", "", "stopping", &now, Actions{}},
		{"failed start", "running", "error", "failed", "error", &now, Actions{Stop: true}},
		{"recoverable error", "stopped", "error", "failed", "error", &now, Actions{Start: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Host{DesiredState: tc.desired, ObservedState: tc.observed, Error: tc.message, LastSeen: tc.seen}
			h.Project(now)
			if h.Status != tc.status || h.Actions != tc.actions {
				t.Fatalf("got %s %+v; want %s %+v", h.Status, h.Actions, tc.status, tc.actions)
			}
		})
	}
}
