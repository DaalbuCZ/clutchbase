package runner

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

type controller interface {
	Status(context.Context) protocol.Report
	Apply(context.Context, protocol.Desired) error
}

type fleetEntry struct {
	runner        controller
	release       func()
	lastError     string
	removed       bool
	whitelistHash string
	appliedPreset *string
}

// Fleet owns locally derived instance names, never API-provided paths or commands.
type Fleet struct {
	resources           resourceSampler
	manager             *manager.Manager
	simulate            bool
	entries             map[string]*fleetEntry
	release             func()
	supportsResultStats bool
	buildMu             sync.Mutex
	latestBuild         string
	buildCheckedAt      time.Time
	buildAttemptAt      time.Time
	buildChecking       bool
	buildCancel         context.CancelFunc
	buildWG             sync.WaitGroup
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
		if !serverID.MatchString(s.ID) || seen[s.ID] || !protocol.ValidInstalledMap(s.Map) || (s.State != "running" && s.State != "stopped") || s.Port < 1024 || s.Port > 65533 || (s.Remove && s.State != "stopped") {
			return errors.New("backend returned invalid server instructions")
		}
		if s.Preset != nil && !protocol.ValidPreset(*s.Preset) {
			return errors.New("unsupported preset")
		}
		if s.ResultAck != nil && !s.ResultAck.Valid() {
			return errors.New("backend returned an invalid match result acknowledgement")
		}
		if s.Restart != nil && (f.simulate || s.Remove || !s.Restart.Valid()) {
			return errors.New("backend returned invalid restart instructions")
		}
		if s.Whitelist != nil {
			if err := protocol.ValidateWhitelist(*s.Whitelist); err != nil {
				return err
			}
		}
		seen[s.ID] = true
		if entry := f.entries[s.ID]; entry != nil && entry.removed && !s.Remove {
			return errors.New("backend attempted to restart a removed instance")
		}
	}
	// Only a completed removal may release a controller omitted by the API.
	for id, entry := range f.entries {
		if !seen[id] && !entry.removed {
			return errors.New("backend omitted an existing server; retaining local controllers")
		}
	}
	for id, entry := range f.entries {
		if !seen[id] {
			entry.release()
			delete(f.entries, id)
		}
	}
	f.supportsResultStats = desired.SupportsResultStats
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
		var applyErr error
		var ackErr error
		if s.ResultAck != nil && !f.simulate {
			ackErr = f.manager.AcknowledgeMatchResult("s-"+strings.ToLower(s.ID), *s.ResultAck)
		}
		if s.Remove {
			if !entry.removed {
				if managed, ok := entry.runner.(*Managed); ok {
					if managed.Manager.PendingMatchResult(managed.Name, true) != nil {
						applyErr = errors.New("a match result is awaiting acknowledgement")
					} else {
						managed.Close()
						applyErr = managed.Manager.Retire(ctx, managed.Name)
					}
				} else if entry.runner.Status(ctx).State != "stopped" {
					applyErr = errors.New("stop the simulated instance before removing it")
				}
				entry.removed = applyErr == nil
			}
		} else {
			applyErr = entry.runner.Apply(ctx, s.Desired)
			if f.simulate && applyErr == nil {
				entry.appliedPreset = nil
				if s.Preset != nil {
					value := *s.Preset
					entry.appliedPreset = &value
				}
				entry.whitelistHash = ""
				if s.Whitelist != nil {
					entry.whitelistHash = protocol.WhitelistHash(*s.Whitelist)
				}
			}
		}
		if err := applyErr; err != nil {
			entry.lastError = err.Error()
			if len(entry.lastError) > 1000 {
				entry.lastError = entry.lastError[:1000]
			}
		} else if ackErr != nil {
			entry.lastError = "Cannot acknowledge the match result"
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
	// ManualReady is a per-instance runtime capability. It is reported only
	// from fresh evidence emitted by the reporter that is actually running.
	out := protocol.MachineReport{Driver: driver, SupportsRestart: !f.simulate, SupportsRemoval: true, SupportsWhitelist: true, SupportsPresets: true, SupportsLiveMode: !f.simulate, SupportsMatchSetup: !f.simulate, SupportsMatchSetupExpiry: !f.simulate, SupportsMatchCancel: !f.simulate, SupportsMatchControl: !f.simulate, SupportsMapObservation: !f.simulate, SupportsReporterUpdate: !f.simulate, SupportsGameUpdate: !f.simulate, Servers: []protocol.ServerReport{}}
	if !f.simulate {
		out.Storage = storageAt(f.manager.Root)
		out.Resources = f.resources.sample("/proc", time.Now())
	}
	ids := make([]string, 0, len(f.entries))
	for id := range f.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	busy := false
	for _, id := range ids {
		entry := f.entries[id]
		report := protocol.Report{Driver: driver, State: "error"}
		if entry.removed {
			report.State = "removed"
		} else if entry.runner != nil {
			report = entry.runner.Status(ctx)
		}
		if f.simulate {
			report.WhitelistHash = entry.whitelistHash
			report.AppliedPreset = entry.appliedPreset
		}
		if !f.simulate {
			if report.State == "installing" || report.GameUpdateStatus == "pending" {
				busy = true
			}
			f.buildMu.Lock()
			latest, checked := f.latestBuild, f.buildCheckedAt
			f.buildMu.Unlock()
			if report.State != "installing" && report.GameUpdateStatus != "pending" && !checked.IsZero() && time.Since(checked) < 2*time.Hour {
				installed := f.manager.InstalledBuild("s-" + strings.ToLower(id))
				if installed != "" {
					report.GameBuildInstalled = installed
					report.GameBuildLatest = latest
					report.GameBuildCheckedAt = checked.UnixMilli()
				}
			}
			report.MatchResult = f.manager.PendingMatchResult("s-"+strings.ToLower(id), f.supportsResultStats)
		}
		if entry.lastError != "" {
			report.Error = entry.lastError
		}
		out.Servers = append(out.Servers, protocol.ServerReport{ID: id, Report: report})
	}
	if !f.simulate && !busy && len(ids) > 0 {
		f.scheduleBuildCheck()
	}
	return out
}

func (f *Fleet) scheduleBuildCheck() {
	f.buildMu.Lock()
	if f.buildChecking || (!f.buildAttemptAt.IsZero() && time.Since(f.buildAttemptAt) < time.Hour) {
		f.buildMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.buildCancel = cancel
	f.buildChecking = true
	f.buildAttemptAt = time.Now()
	f.buildWG.Add(1)
	f.buildMu.Unlock()
	go func() {
		defer f.buildWG.Done()
		latest, err := f.manager.PublicBuild(ctx)
		f.buildMu.Lock()
		if err == nil {
			f.latestBuild = latest
			f.buildCheckedAt = time.Now()
		} else {
			f.latestBuild = ""
			f.buildCheckedAt = time.Time{}
		}
		f.buildChecking = false
		f.buildMu.Unlock()
	}()
}

func (f *Fleet) ConnectionFailed() { f.supportsResultStats = false }

func (f *Fleet) Close() {
	f.buildMu.Lock()
	if f.buildCancel != nil {
		f.buildCancel()
	}
	f.buildMu.Unlock()
	f.buildWG.Wait()
	for _, entry := range f.entries {
		if m, ok := entry.runner.(*Managed); ok {
			m.Close()
		}
		entry.release()
	}
	f.release()
}
