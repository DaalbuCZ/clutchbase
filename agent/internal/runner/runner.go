package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"clutchbase/agent/internal/gamequery"
	"clutchbase/agent/internal/hostlog"
	"clutchbase/internal/protocol"
)

// Runner only controls a service and config path selected locally by its operator.
type Runner struct {
	Simulate   bool
	Unit       string
	ConfigPath string
	Port       int
	state      string
	activeMap  string
}

var unitName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@-]*\.service$`)

func (r *Runner) Validate() error {
	if r.Simulate {
		return nil
	}
	if !unitName.MatchString(r.Unit) || !filepath.IsAbs(r.ConfigPath) {
		return errors.New("systemd mode requires a valid .service unit and absolute config path")
	}
	return nil
}
func (r *Runner) Status(ctx context.Context) protocol.Report {
	if r.Simulate {
		state := r.state
		if state == "" {
			state = "stopped"
		}
		return protocol.Report{State: state, ActiveMap: r.activeMap, Driver: "simulated"}
	}
	state, err := r.systemctl(ctx, "is-active", r.Unit)
	report := protocol.Report{State: "error", Driver: "systemd"}
	switch state {
	case "active":
		report.State = "running"
		// An active service does not prove CS2 accepts players; the game query
		// does. Until it answers, the instance keeps the starting projection.
		if info, err := gamequery.Query(ctx, r.Port); err != nil {
			report.State = "starting"
		} else {
			report.Players, report.MaxPlayers = &info.Players, &info.MaxPlayers
			if protocol.ValidMap(info.Map) {
				report.ActiveMap = info.Map
			}
		}
	case "inactive":
		report.State = "stopped"
	case "activating":
		report.State = "starting"
	case "deactivating":
		report.State = "stopping"
	case "failed":
		report.Error = "Service failed; inspect journalctl --user -u " + r.Unit
	default:
		if err != nil {
			report.Error = "Cannot inspect service; verify the unit and user systemd session"
		}
	}
	if report.State != "running" {
		report.ActiveMap = ""
	}
	report.LogTail = hostlog.Journal(r.Unit, 30, 4096)
	return report
}
func (r *Runner) Apply(ctx context.Context, desired protocol.Desired) error {
	if desired.ReporterUpdate != nil {
		return errors.New("simulation cannot update a reporter")
	}
	if desired.MatchCancel != nil {
		return errors.New("simulation cannot cancel a match setup")
	}
	if desired.LiveMode != nil {
		return errors.New("simulation cannot switch a running game mode")
	}
	if desired.MatchControl != nil {
		return errors.New("simulation cannot control a live match")
	}
	if !protocol.ValidMap(desired.Map) || (desired.State != "running" && desired.State != "stopped") {
		return errors.New("backend returned an unsupported state or map")
	}
	r.Port = desired.Port
	if r.Simulate {
		r.state = desired.State
		r.activeMap = desired.Map
		return nil
	}
	status := r.Status(ctx)
	if desired.State == "stopped" {
		if status.State == "stopped" || status.State == "stopping" {
			return nil
		}
		_, err := r.systemctl(ctx, "stop", r.Unit)
		return err
	}
	if status.State == "running" || status.State == "starting" {
		return nil
	}
	if err := writeConfig(r.ConfigPath, desired.Map); err != nil {
		return fmt.Errorf("write server map config: %w", err)
	}
	if _, err := r.systemctl(ctx, "start", r.Unit); err != nil {
		return err
	}
	r.activeMap = desired.Map
	return nil
}
func (r *Runner) systemctl(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(string(out)), errors.New("systemctl operation failed; inspect the service journal and agent permissions")
	}
	return strings.TrimSpace(string(out)), nil
}
func writeConfig(path, mapName string) error {
	if !protocol.ValidMap(mapName) {
		return errors.New("unsupported map")
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".clutchbase-*.cfg")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = fmt.Fprintf(file, "// Managed by Clutchbase.\nmap %s\n", mapName); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
