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
	Manager      *manager.Manager
	Name         string
	mu           sync.Mutex
	cancel       context.CancelFunc
	done         chan struct{}
	installError error
	activeMap    string
}

func (m *Managed) Status(ctx context.Context) protocol.Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return protocol.Report{Driver: "managed", State: "installing"}
	}
	report := m.Manager.Status(ctx, m.Name)
	if report.State == "running" {
		report.ActiveMap = m.activeMap
	}
	if m.installError != nil {
		report.State = "error"
		report.Error = m.installError.Error()
	}
	return report
}
func (m *Managed) Apply(ctx context.Context, desired protocol.Desired) error {
	if !protocol.ValidMap(desired.Map) || (desired.State != "running" && desired.State != "stopped") {
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
	if desired.State == "stopped" {
		m.installError = nil
		m.activeMap = ""
		if m.cancel != nil {
			m.cancel()
			return nil
		}
		return m.Manager.Stop(ctx, m.Name)
	}
	if m.cancel != nil {
		return nil
	}
	if m.installError != nil {
		return m.installError
	}
	state := m.Manager.Status(ctx, m.Name)
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
	m.activeMap = desired.Map
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
