package runner

import (
	"context"
	"errors"
	"os"
	"sync"

	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

// Managed installs in the background so heartbeats remain live during downloads.
type Managed struct {
	Manager          *manager.Manager
	Name             string
	mu               sync.Mutex
	cancel           context.CancelFunc
	done             chan struct{}
	installError     error
	whitelistHash    string
	appliedPreset    *string
	reporterID       string
	reporterHash     string
	reporterStatus   string
	reporterError    string
	installKind      string
	gameUpdateID     string
	gameUpdateStatus string
	gameUpdateError  string
	restartStopReady bool
}

func (m *Managed) Status(ctx context.Context) protocol.Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	report := m.Manager.Status(ctx, m.Name)
	if maps, err := m.Manager.WorkshopMaps(m.Name); err == nil {
		report.WorkshopMaps = &maps
	}
	if id, phase, err := m.Manager.RestartFence(m.Name); err == nil && (phase != "stopped" || m.restartStopReady) {
		report.RestartID, report.RestartPhase = id, phase
	}
	if m.cancel != nil {
		report.State = "installing"
	}
	report.WhitelistHash = m.whitelistHash
	report.AppliedPreset = m.appliedPreset
	report.LogTail = m.Manager.LogTail(m.Name)
	report.ReporterUpdateID = m.reporterID
	report.ReporterUpdateStatus = m.reporterStatus
	report.ReporterUpdateError = m.reporterError
	report.GameUpdateID = m.gameUpdateID
	report.GameUpdateStatus = m.gameUpdateStatus
	report.GameUpdateError = m.gameUpdateError
	if m.installError != nil {
		report.State = "error"
		report.Error = m.installError.Error()
	}
	return report
}
func (m *Managed) Apply(ctx context.Context, desired protocol.Desired) error {
	if desired.Restart != nil {
		return m.applyRestart(ctx, desired, desired.Restart)
	}
	if desired.GameUpdate != nil && (!protocol.ValidCommandID(desired.GameUpdate.ID) || desired.State != "stopped") {
		return errors.New("backend returned an invalid game update")
	}
	if desired.ReporterUpdate != nil && (!protocol.ValidCommandID(desired.ReporterUpdate.ID) || !protocol.ValidReporterHash(desired.ReporterUpdate.Hash) || desired.State != "stopped") {
		return errors.New("backend returned an invalid reporter update")
	}
	if desired.Preset != nil && !protocol.ValidPreset(*desired.Preset) {
		return errors.New("unsupported preset")
	}
	if desired.LiveMode != nil && (!protocol.ValidCommandID(desired.LiveMode.ID) || (desired.LiveMode.Mode != "match" && desired.LiveMode.Mode != "practice")) {
		return errors.New("backend returned an invalid live mode action")
	}
	if desired.MatchSetup != nil && (!protocol.ValidCommandID(desired.MatchSetup.ID) || !protocol.ValidInstalledMap(desired.MatchSetup.Map)) {
		return errors.New("backend returned an invalid MatchZy match setup")
	}
	if desired.MatchControl != nil && (!protocol.ValidCommandID(desired.MatchControl.ID) || !protocol.ValidCommandID(desired.MatchControl.SetupID) || (desired.MatchControl.Action != "pause" && desired.MatchControl.Action != "resume" && desired.MatchControl.Action != "end")) {
		return errors.New("backend returned an invalid match control action")
	}
	if desired.MatchCancel != nil && (!protocol.ValidCommandID(desired.MatchCancel.ID) || !protocol.ValidCommandID(desired.MatchCancel.SetupID)) {
		return errors.New("backend returned an invalid match cancellation")
	}
	if desired.Whitelist != nil {
		if err := protocol.ValidateWhitelist(*desired.Whitelist); err != nil {
			return err
		}
	}
	if !protocol.ValidInstalledMap(desired.Map) || (desired.State != "running" && desired.State != "stopped") {
		return errors.New("backend returned an unsupported state or map")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	in, err := m.Manager.Load(m.Name)
	if errors.Is(err, os.ErrNotExist) {
		in, err = m.Manager.Create(manager.Instance{Name: m.Name, Port: desired.Port, Map: desired.Map, Password: os.Getenv("CLUTCHBASE_SERVER_PASSWORD"), GSLT: os.Getenv("CLUTCHBASE_GSLT")})
	}
	if err != nil {
		return err
	}
	if in.Port != desired.Port {
		return errors.New("registered port differs from local instance; register the correct game port")
	}
	if _, phase, err := m.Manager.RestartFence(m.Name); err == nil && (phase == "stopping" || phase == "stopped" || phase == "starting") && desired.State == "running" {
		return errors.New("pending restart requires an explicit matching start command")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("restart state is unavailable")
	}
	if desired.State == "stopped" {
		m.installError = nil
		if m.cancel != nil {
			if m.installKind == "game-update" {
				return nil
			}
			m.cancel()
			return nil
		}
		if desired.GameUpdate != nil {
			return m.updateGame(ctx, desired.GameUpdate.ID)
		}
		if desired.ReporterUpdate != nil {
			command := desired.ReporterUpdate
			if m.reporterID == command.ID && m.reporterHash != command.Hash {
				m.reporterStatus, m.reporterError = "failed", "Reporter update ID was reused"
				return nil
			}
			m.reporterID, m.reporterHash = command.ID, command.Hash
			if err := m.Manager.UpdateReporter(ctx, m.Name, command.Hash); err != nil {
				m.reporterStatus, m.reporterError = "failed", err.Error()
			} else {
				m.reporterStatus, m.reporterError = "complete", ""
			}
			return nil
		}
		if err := m.Manager.Stop(ctx, m.Name); err != nil {
			return err
		}
		if err := m.syncWhitelist(ctx, desired); err != nil {
			return err
		}
		return m.syncPreset(ctx, desired)
	}
	if m.cancel != nil {
		return nil
	}
	if m.installError != nil {
		return m.installError
	}
	if err := m.syncWhitelist(ctx, desired); err != nil {
		return err
	}
	if err := m.syncPreset(ctx, desired); err != nil {
		return err
	}
	state := m.Manager.Status(ctx, m.Name)
	if desired.MatchCancel != nil {
		return m.Manager.RequestMatchCancellation(ctx, m.Name, *desired.MatchCancel)
	}
	if desired.LiveMode != nil {
		if state.State != "running" {
			return errors.New("server must be running to change its live mode")
		}
		return m.Manager.RequestLiveMode(ctx, m.Name, *desired.LiveMode)
	}
	if desired.MatchSetup != nil {
		if state.State != "running" {
			return errors.New("server must be running to load a MatchZy match")
		}
		return m.Manager.RequestMatchSetup(ctx, m.Name, *desired.MatchSetup)
	}
	if desired.MatchControl != nil {
		if state.State != "running" {
			return errors.New("server must be running to control a match")
		}
		return m.Manager.RequestMatchControl(ctx, m.Name, *desired.MatchControl)
	}
	if state.AccessStatus == "blocked" {
		return errors.New("runtime whitelist verification failed; stop the instance before retrying")
	}
	if state.State == "uninstalled" {
		job, cancel := context.WithCancel(ctx)
		m.cancel = cancel
		m.done = make(chan struct{})
		done := m.done
		go func() {
			err := m.Manager.Install(job, m.Name, false)
			cancel()
			m.mu.Lock()
			defer m.mu.Unlock()
			m.cancel = nil
			if !errors.Is(err, context.Canceled) {
				m.installError = err
			}
			close(done)
		}()
		return nil
	}
	if state.State == "running" || state.State == "starting" {
		return nil
	}
	if state.State == "installing" {
		return nil
	}
	if err := m.Manager.Start(ctx, m.Name, desired.Map); err != nil {
		return err
	}
	return nil
}

func (m *Managed) applyRestart(ctx context.Context, desired protocol.Desired, command *protocol.RestartCommand) error {
	if !command.Valid() {
		return errors.New("backend returned an invalid restart command")
	}
	if desired.State != "running" && desired.State != "stopped" {
		return errors.New("unsupported desired state")
	}
	if !protocol.ValidInstalledMap(desired.Map) || desired.GameUpdate != nil || desired.ReporterUpdate != nil || desired.MatchSetup != nil || desired.MatchControl != nil || desired.MatchCancel != nil || desired.LiveMode != nil {
		return errors.New("restart cannot be combined with another operation")
	}
	if desired.Preset != nil && !protocol.ValidPreset(*desired.Preset) {
		return errors.New("unsupported preset")
	}
	if desired.Whitelist != nil {
		if err := protocol.ValidateWhitelist(*desired.Whitelist); err != nil {
			return err
		}
	}
	if (command.Phase == "stop" || command.Phase == "cancel") && desired.State != "stopped" {
		return errors.New("restart stop or cancellation requires stopped desired state")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return errors.New("server installation or update is in progress")
	}
	in, err := m.Manager.Load(m.Name)
	if err != nil {
		return err
	}
	if in.Port != desired.Port {
		return errors.New("registered port differs from local instance")
	}
	if command.Phase == "cancel" {
		m.restartStopReady = false
		_, _, err := m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "cancel")
		if err != nil {
			return err
		}
		return m.Manager.Stop(ctx, m.Name)
	}
	if command.Phase == "stop" {
		_, phase, err := m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "begin")
		if err != nil {
			return err
		}
		if phase == "stopped" {
			if err := m.syncWhitelist(ctx, desired); err != nil {
				return err
			}
			if err := m.syncPreset(ctx, desired); err != nil {
				return err
			}
			m.restartStopReady = true
			return nil
		}
		if phase != "stopping" {
			return nil
		}
		state := m.Manager.Status(ctx, m.Name)
		if state.State != "stopped" {
			if err := m.Manager.Stop(ctx, m.Name); err != nil {
				_, _, _ = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "failed")
				return err
			}
			state = m.Manager.Status(ctx, m.Name)
		}
		if state.State == "stopped" {
			if err := m.syncWhitelist(ctx, desired); err != nil {
				return err
			}
			if err := m.syncPreset(ctx, desired); err != nil {
				return err
			}
			_, _, err = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "stopped")
			if err == nil {
				m.restartStopReady = true
			}
			return err
		}
		return nil
	}
	id, phase, err := m.Manager.RestartFence(m.Name)
	if err != nil || id != command.ID || desired.State != "running" {
		return errors.New("restart start is not authorized by the stopped request")
	}
	if phase == "starting" {
		state := m.Manager.Status(ctx, m.Name).State
		if state == "stopped" {
			if err := m.syncWhitelist(ctx, desired); err != nil {
				return err
			}
			if err := m.syncPreset(ctx, desired); err != nil {
				return err
			}
			if err := m.Manager.Start(ctx, m.Name, desired.Map); err != nil {
				_, _, _ = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "failed")
				return err
			}
		} else if state == "running" {
			_, _, err = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "running")
		}
		return err
	}
	if phase != "stopped" {
		return errors.New("restart start is not authorized by the stopped request")
	}
	if m.Manager.Status(ctx, m.Name).State != "stopped" {
		return errors.New("server is not stopped")
	}
	if err := m.syncWhitelist(ctx, desired); err != nil {
		return err
	}
	if err := m.syncPreset(ctx, desired); err != nil {
		return err
	}
	_, _, err = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "start_authorized")
	if err != nil {
		return err
	}
	if err = m.Manager.Start(ctx, m.Name, desired.Map); err != nil {
		_, _, _ = m.Manager.RecordRestartFence(ctx, m.Name, command.ID, "failed")
		return err
	}
	return err
}

func (m *Managed) updateGame(ctx context.Context, id string) error {
	if m.gameUpdateID == id {
		return nil
	}
	state := m.Manager.Status(ctx, m.Name)
	if state.State != "stopped" {
		return errors.New("stop the server before updating CS2")
	}
	m.gameUpdateID, m.gameUpdateStatus, m.gameUpdateError = id, "pending", ""
	job, cancel := context.WithCancel(ctx)
	m.cancel, m.installKind, m.done = cancel, "game-update", make(chan struct{})
	done := m.done
	go func() {
		err := m.Manager.Install(job, m.Name, true)
		cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cancel, m.installKind = nil, ""
		if errors.Is(err, context.Canceled) {
			m.gameUpdateStatus, m.gameUpdateError = "pending", ""
		} else if err != nil {
			m.gameUpdateStatus = "failed"
			m.gameUpdateError = err.Error()
			if len(m.gameUpdateError) > 200 {
				m.gameUpdateError = m.gameUpdateError[:200]
			}
		} else {
			m.gameUpdateStatus, m.gameUpdateError = "complete", ""
		}
		close(done)
	}()
	return nil
}
func (m *Managed) Close() {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	if cancel != nil {
		cancel()
	}
	m.mu.Unlock()
	if cancel != nil {
		<-done
	}
}

func (m *Managed) syncWhitelist(ctx context.Context, desired protocol.Desired) error {
	if desired.Whitelist == nil {
		m.whitelistHash = ""
		return nil
	}
	m.whitelistHash = ""
	if err := m.Manager.SyncWhitelist(ctx, m.Name, *desired.Whitelist); err != nil {
		return err
	}
	m.whitelistHash = protocol.WhitelistHash(*desired.Whitelist)
	return nil
}

func (m *Managed) syncPreset(ctx context.Context, desired protocol.Desired) error {
	m.appliedPreset = nil
	if desired.Preset == nil {
		return nil
	}
	if err := m.Manager.SyncPreset(ctx, m.Name, *desired.Preset); err != nil {
		return err
	}
	value := *desired.Preset
	m.appliedPreset = &value
	return nil
}
