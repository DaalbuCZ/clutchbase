package runner

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"

	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

type controller interface {
	Status(context.Context) protocol.Report
	Apply(context.Context, protocol.Desired) error
}

type fleetEntry struct {
	runner    controller
	release   func()
	lastError string
}

// Fleet owns locally derived instance names, never API-provided paths or commands.
type Fleet struct {
	manager  *manager.Manager
	simulate bool
	entries  map[string]*fleetEntry
	release  func()
}

func NewFleet(m *manager.Manager, simulate bool) (*Fleet, error) {
	release, err := m.AcquireAgent("fleet")
	if err != nil {
		return nil, err
	}
	return &Fleet{manager: m, simulate: simulate, entries: map[string]*fleetEntry{}, release: release}, nil
}

var serverID = regexp.MustCompile(`^[A-Z2-7]{26}$`)

func (f *Fleet) Apply(ctx context.Context, desired protocol.MachineDesired) error {
	if len(desired.Servers) > protocol.MaxServers {
		return errors.New("backend exceeded machine server limit")
	}
	seen := map[string]bool{}
	for _, s := range desired.Servers {
		if !serverID.MatchString(s.ID) || seen[s.ID] || !protocol.ValidMap(s.Map) || (s.State != "running" && s.State != "stopped") || s.Port < 1024 || s.Port > 65533 {
			return errors.New("backend returned invalid server instructions")
		}
		seen[s.ID] = true
	}
	// Deletion is not supported: a missing row must not silently orphan a controller.
	for id := range f.entries {
		if !seen[id] {
			return errors.New("backend omitted an existing server; retaining local controllers")
		}
	}
	for _, s := range desired.Servers {
		entry := f.entries[s.ID]
		if entry == nil || entry.runner == nil {
			if f.simulate {
				entry = &fleetEntry{runner: &Runner{Simulate: true}, release: func() {}}
			} else {
				m := *f.manager
				name := "s-" + strings.ToLower(s.ID)
				release, err := m.AcquireAgent(name)
				if err != nil {
					// Retry acquiring this instance on the next heartbeat, without blocking siblings.
					f.entries[s.ID] = &fleetEntry{lastError: "Cannot acquire instance controller; stop its other agent", release: func() {}}
					continue
				}
				entry = &fleetEntry{runner: &Managed{Manager: &m, Name: name}, release: release}
			}
			f.entries[s.ID] = entry
		}
		if err := entry.runner.Apply(ctx, s.Desired); err != nil {
			entry.lastError = err.Error()
			if len(entry.lastError) > 1000 {
				entry.lastError = entry.lastError[:1000]
			}
		} else {
			entry.lastError = ""
		}
	}
	return nil
}

func (f *Fleet) Report(ctx context.Context) protocol.MachineReport {
	driver := "managed"
	if f.simulate {
		driver = "simulated"
	}
	out := protocol.MachineReport{Driver: driver, Servers: []protocol.ServerReport{}}
	ids := make([]string, 0, len(f.entries))
	for id := range f.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entry := f.entries[id]
		report := protocol.Report{Driver: driver, State: "error"}
		if entry.runner != nil {
			report = entry.runner.Status(ctx)
		}
		if entry.lastError != "" {
			report.Error = entry.lastError
		}
		out.Servers = append(out.Servers, protocol.ServerReport{ID: id, Report: report})
	}
	return out
}

func (f *Fleet) Close() {
	for _, entry := range f.entries {
		if m, ok := entry.runner.(*Managed); ok {
			m.Close()
		}
		entry.release()
	}
	f.release()
}
