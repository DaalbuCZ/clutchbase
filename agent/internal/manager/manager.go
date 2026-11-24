// Package manager provisions independent CS2 instances without privileged commands.
package manager

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"clutchbase/agent/internal/access"
	"clutchbase/agent/internal/gamequery"
	"clutchbase/agent/internal/hostlog"
	"clutchbase/internal/protocol"
)

type Manager struct {
	Root, UnitDir, SteamCMD string
	PresetPlugin            string
	Guard                   string
	agentName               string
}

// Instance is private on-disk configuration; never return it through the API.
type Instance struct {
	Preset    string   `json:"preset,omitempty"`
	Name      string   `json:"name"`
	Port      int      `json:"port"`
	Map       string   `json:"map"`
	Password  string   `json:"password"`
	GSLT      string   `json:"gslt,omitempty"`
	Whitelist []string `json:"whitelist,omitempty"`
}
type Info struct {
	Name  string `json:"name"`
	Port  int    `json:"port"`
	Map   string `json:"map"`
	State string `json:"state"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
var tokenPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var ErrBusy = errors.New("instance is busy; wait for installation or cancel it through its agent")

func New(root, steamcmd string) (*Manager, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, ".local", "share", "clutchbase")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(root, "\r\n\x00") {
		return nil, errors.New("invalid instance root")
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	if steamcmd == "" {
		steamcmd = "steamcmd"
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return &Manager{Root: root, UnitDir: filepath.Join(config, "systemd", "user"), SteamCMD: steamcmd, Guard: filepath.Join(filepath.Dir(executable), "clutchbase-guard"), PresetPlugin: filepath.Join(filepath.Dir(executable), "ClutchbaseAccess")}, nil
}
func (m *Manager) Dir(name string) (string, error) {
	if !namePattern.MatchString(name) {
		return "", errors.New("instance name must start with a lowercase letter and contain at most 32 lowercase letters, digits or hyphens")
	}
	return filepath.Join(m.Root, "instances", name), nil
}

// A linked instance has one controller; local CLI mutations cannot fight the API.
func (m *Manager) lease(name string) (func(), error) {
	if _, err := m.Dir(name); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.Root, 0700); err != nil {
		return nil, err
	}
	unlock, err := lock(filepath.Join(m.Root, "agent-"+name+".lock"))
	if errors.Is(err, ErrBusy) {
		return nil, errors.New("instance has an active agent; manage it through the dashboard")
	}
	return unlock, err
}
func (m *Manager) AcquireAgent(name string) (func(), error) {
	unlock, err := m.lease(name)
	if err != nil {
		return nil, err
	}
	m.agentName = name
	return unlock, nil
}
func (m *Manager) access(name string) (func(), error) {
	if m.agentName == name {
		return func() {}, nil
	}
	return m.lease(name)
}
func unit(name string) string { return "clutchbase-" + name + ".service" }
func safeValue(value string) bool {
	if len(value) > 128 {
		return false
	}
	for _, c := range value {
		if c < 32 || c == 127 || strings.ContainsRune("\"\\;", c) {
			return false
		}
	}
	return true
}
func (m *Manager) Load(name string) (Instance, error) {
	var in Instance
	dir, err := m.Dir(name)
	if err != nil {
		return in, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		return in, err
	}
	if err = json.Unmarshal(data, &in); err != nil {
		return in, err
	}
	if in.Name != name || in.Port < 1024 || in.Port > 65533 || !protocol.ValidInstalledMap(in.Map) || !protocol.ValidPreset(in.Preset) || !safeValue(in.Password) || (in.GSLT != "" && !tokenPattern.MatchString(in.GSLT)) {
		return in, errors.New("invalid local instance configuration")
	}
	if err := validateWhitelist(in.Whitelist); err != nil {
		return in, err
	}
	return in, nil
}
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func waitLock(ctx context.Context, path string) (func(), error) {
	for {
		unlock, err := lock(path)
		if !errors.Is(err, ErrBusy) {
			return unlock, err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
func (m *Manager) Create(in Instance) (Instance, error) {
	dir, err := m.Dir(in.Name)
	if err != nil {
		return in, err
	}
	if in.Port < 1024 || in.Port > 65533 || !protocol.ValidInstalledMap(in.Map) || !protocol.ValidPreset(in.Preset) || !safeValue(in.Password) || (in.GSLT != "" && !tokenPattern.MatchString(in.GSLT)) {
		return in, errors.New("use an official map, a port from 1024 to 65533, a safe password and an optional 32-character hex GSLT")
	}
	if in.Password == "" {
		in.Password = rand.Text()
	}
	if len(in.Whitelist) != 0 || in.Preset != "" {
		return in, errors.New("configure the whitelist after installing the instance and plugins")
	}
	if err := m.checkUnit(in.Name); err != nil {
		return in, err
	}
	if err = os.MkdirAll(filepath.Join(m.Root, "instances"), 0700); err != nil {
		return in, err
	}
	unlock, err := lock(filepath.Join(m.Root, "create.lock"))
	if err != nil {
		return in, err
	}
	defer unlock()
	entries, err := os.ReadDir(filepath.Join(m.Root, "instances"))
	if err != nil {
		return in, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		existing, err := m.Load(entry.Name())
		if err != nil {
			return in, fmt.Errorf("inspect existing instance %s: %w", entry.Name(), err)
		}
		if in.Port <= existing.Port+2 && existing.Port <= in.Port+2 {
			return in, fmt.Errorf("ports %d–%d overlap instance %s", in.Port, in.Port+2, existing.Name)
		}
		if in.GSLT != "" && in.GSLT == existing.GSLT {
			return in, errors.New("each instance needs a different GSLT")
		}
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return in, err
	}
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return in, err
	}
	if err = atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		_ = os.Remove(dir)
		return in, err
	}
	return in, nil
}
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".clutchbase-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (m *Manager) installed(name string) bool {
	dir, err := m.Dir(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "installed"))
	if err != nil {
		return false
	}
	if _, err = os.Stat(filepath.Join(m.UnitDir, unit(name))); err != nil || m.checkUnit(name) != nil {
		return false
	}
	return gameFilesPresent(filepath.Join(dir, "server"))
}
func gameFilesPresent(serverDir string) bool {
	for _, path := range []string{filepath.Join("game", "cs2.sh"), filepath.Join("game", "bin", "linuxsteamrt64", "cs2")} {
		file, err := os.Stat(filepath.Join(serverDir, path))
		if err != nil || file.IsDir() || file.Mode()&0111 == 0 {
			return false
		}
	}
	return true
}
func (m *Manager) Status(ctx context.Context, name string) protocol.Report {
	report := protocol.Report{Driver: "managed", State: "error"}
	in, err := m.Load(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report.State = "uninstalled"
		} else {
			report.Error = "Cannot read local instance configuration"
		}
		return report
	}
	report.JoinPassword = in.Password
	dir, _ := m.Dir(name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if errors.Is(err, ErrBusy) {
		if _, err := os.Stat(filepath.Join(dir, "installing")); err == nil {
			report.State = "installing"
			return report
		}
	} else if err != nil {
		report.Error = "Cannot lock local instance"
		return report
	} else {
		unlock()
	}
	if !m.installed(name) {
		report.State = "uninstalled"
		return report
	}
	report.ReporterSourceHash, report.ReporterInstalledHash = m.reporterHashes(name)
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.State = state
	report.AccessStatus = "password"
	if len(in.Whitelist) != 0 {
		report.AccessStatus = "pending"
		if access.Failure(m.accessDir(in.Name)) || state == "error" {
			report.AccessStatus = "blocked"
			report.State = "error"
			report.Error = "Runtime whitelist verification failed; stop the instance before retrying"
		} else if state == "running" {
			var request access.Request
			report.State = "starting"
			if access.Read(filepath.Join(m.accessDir(in.Name), "request.json"), &request) == nil && request.RosterHash == access.Hash([]byte(strings.Join(in.Whitelist, "\n")+"\n")) {
				if evidence, err := access.Verified(m.accessDir(in.Name), request, time.Now()); err == nil {
					report.AccessStatus = "verified"
					report.State = "running"
					if protocol.ValidPreset(evidence.Mode) {
						report.ActivePreset = evidence.Mode
					}
					report.CanLiveMode = evidence.CanSwitchMode
					report.CanLoadMatch = evidence.CanLoadMatch
					// A capability bit from a reporter is meaningful only when its
					// current MatchZy runtime is recognized and match setup is usable.
					report.SupportsManualReady = evidence.SupportsManualReady && evidence.CanLoadMatch
					report.SupportsMatchSeries = evidence.SupportsMatchSeries && evidence.CanLoadMatch
					report.SupportsFencedMatchControl = evidence.SupportsFencedMatchControl
					report.MatchPhase = evidence.MatchPhase
					if evidence.LiveScore.Valid() && evidence.LiveScore.ObservedAt == evidence.ReportedAt {
						report.LiveScore = evidence.LiveScore
					}
					report.ActiveMatchSetupID = evidence.ActiveMatchSetupID
					report.SupportsMatchFence = evidence.SupportsMatchFence
					report.CanPauseMatch = evidence.CanPauseMatch
					report.CanResumeMatch = evidence.CanResumeMatch
					if evidence.ModeCommandStatus == "complete" || evidence.ModeCommandStatus == "failed" {
						report.ModeCommandID = evidence.ModeCommandID
						report.ModeCommandStatus = evidence.ModeCommandStatus
						report.ModeCommandError = evidence.ModeCommandError
					}
					if evidence.MatchSetupStatus == "complete" || evidence.MatchSetupStatus == "failed" {
						report.MatchSetupID = evidence.MatchSetupID
						report.MatchSetupStatus = evidence.MatchSetupStatus
						report.MatchSetupError = evidence.MatchSetupError
					}
					if evidence.MatchControlStatus == "complete" || evidence.MatchControlStatus == "failed" {
						report.MatchControlID = evidence.MatchControlID
						report.MatchControlAction = evidence.MatchControlAction
						report.MatchControlStatus = evidence.MatchControlStatus
						report.MatchControlError = evidence.MatchControlError
					}
					if evidence.MatchCancelStatus == "complete" || evidence.MatchCancelStatus == "failed" {
						report.MatchCancelID = evidence.MatchCancelID
						report.MatchCancelSetupID = evidence.MatchCancelSetupID
						report.MatchCancelStatus = evidence.MatchCancelStatus
						report.MatchCancelError = evidence.MatchCancelError
					}
				}
			}
		}
	}
	if report.State == "running" {
		// An active service does not prove CS2 accepts players; the game query
		// does. Until it answers, the instance keeps the starting projection.
		info, err := gamequery.Query(ctx, in.Port)
		if err != nil {
			report.State = "starting"
		} else {
			report.Players, report.MaxPlayers = &info.Players, &info.MaxPlayers
			if protocol.ValidInstalledMap(info.Map) {
				report.ActiveMap = info.Map
			}
		}
	}
	if report.LiveScore != nil && (report.State != "running" || report.AccessStatus != "verified" || report.ActivePreset != "match" || report.LiveScore.SetupID != report.ActiveMatchSetupID || report.LiveScore.Map != report.ActiveMap || (report.MatchPhase != "live" && report.MatchPhase != "paused")) {
		report.LiveScore = nil
	}
	if state == "error" && report.Error == "" {
		report.Error = "Service failed; inspect journalctl --user -u " + unit(name)
	}
	return report
}

// LogTail returns a redacted, capped tail of the instance's service journal and
// install log for dashboard display. Missing sources are simply omitted.
func (m *Manager) LogTail(name string) string {
	dir, err := m.Dir(name)
	if err != nil {
		return ""
	}
	const separator = "\n--- install.log ---\n"
	journal := hostlog.Journal(unit(name), 30, 2048)
	install := hostlog.TailFile(filepath.Join(dir, "install.log"), 2048-int64(len(separator)))
	switch {
	case journal == "":
		return install
	case install == "":
		return journal
	default:
		return journal + separator + install
	}
}
func serviceState(ctx context.Context, name string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "--user", "is-active", name).Output()
	switch strings.TrimSpace(string(out)) {
	case "active":
		return "running", nil
	case "activating":
		return "starting", nil
	case "deactivating":
		return "stopping", nil
	case "inactive", "unknown":
		return "stopped", nil
	case "failed":
		return "error", nil
	}
	if err != nil {
		return "error", errors.New("cannot query user systemd; check the user session and service journal")
	}
	return "error", errors.New("unexpected systemd state")
}
func systemctl(ctx context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if len(args) > 0 && (args[0] == "start" || args[0] == "stop") {
		args = append([]string{"--no-block"}, args...)
	}
	if err := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...).Run(); err != nil {
		return errors.New("systemctl failed; check the user session and service journal")
	}
	return nil
}

// Install never modifies an active instance. An interrupted download has no ready marker.
func (m *Manager) Install(ctx context.Context, name string, update bool) error {
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	in, err := m.Load(name)
	if err != nil {
		return err
	}
	dir, _ := m.Dir(name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if err = m.checkUnit(name); err != nil {
		return err
	}
	if m.installed(name) && !update {
		return nil
	}
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state != "stopped" && state != "error" {
		return errors.New("stop the instance before installing or updating it")
	}
	if _, err := exec.LookPath(m.SteamCMD); err != nil {
		return errors.New("SteamCMD not found; install it on the host or set --steamcmd")
	}
	if err = atomicWrite(filepath.Join(dir, "installing"), nil); err != nil {
		return err
	}
	defer os.Remove(filepath.Join(dir, "installing"))
	if err = os.Remove(filepath.Join(dir, "installed")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	serverDir := filepath.Join(dir, "server")
	if err = os.MkdirAll(serverDir, 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "install.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	args := []string{"+force_install_dir", serverDir, "+login", "anonymous", "+app_update", "730"}
	if !update {
		args = append(args, "validate")
	}
	args = append(args, "+quit")
	job, cancel := context.WithTimeout(ctx, 4*time.Hour)
	defer cancel()
	// SteamCMD also updates its own runtime; serialize its processes within a root.
	unlockSteam, err := waitLock(job, filepath.Join(m.Root, "steamcmd.lock"))
	if err != nil {
		return err
	}
	defer unlockSteam()
	cmd := exec.CommandContext(job, m.SteamCMD, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	if err = cmd.Run(); err != nil {
		if job.Err() != nil {
			return job.Err()
		}
		return errors.New("SteamCMD failed; inspect the instance install.log")
	}
	if !gameFilesPresent(serverDir) {
		return errors.New("SteamCMD did not produce the CS2 launcher and server binary; inspect install.log")
	}
	if err = m.writeConfig(in, in.Map); err != nil {
		return err
	}
	if err = m.writeUnit(in); err != nil {
		return err
	}
	if err = systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	check, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	defer checkCancel()
	loaded, err := exec.CommandContext(check, "systemctl", "--user", "show", "--property=LoadState", "--value", unit(name)).Output()
	if err != nil || strings.TrimSpace(string(loaded)) != "loaded" {
		return errors.New("generated systemd unit did not load; inspect the service journal")
	}
	return atomicWrite(filepath.Join(dir, "installed"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"))
}
func (m *Manager) writeConfig(in Instance, mapName string) error {
	if err := m.CheckInstalledMap(in.Name, mapName); err != nil {
		return err
	}
	if !protocol.ValidInstalledMap(mapName) {
		return errors.New("unsupported map")
	}
	if in.Preset != "" {
		if err := m.checkPresetReporter(in.Name); err != nil {
			return err
		}
	}
	if err := m.writeWhitelist(in); err != nil {
		return err
	}
	dir, _ := m.Dir(in.Name)
	cfgDir := filepath.Join(dir, "server", "game", "csgo", "cfg")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return err
	}
	config := fmt.Sprintf("// Managed by Clutchbase.\nhostname \"%s\"\nsv_password \"%s\"\nsv_lan 0\ntv_enable 0\ntv_port %d\n", in.Name, in.Password, in.Port+2)
	// CS2 resets game cvars on map load, then executes server.cfg. Keep map
	// commands out of it so reapplying the private settings cannot reload the map.
	if err := atomicWrite(filepath.Join(cfgDir, "server.cfg"), []byte(config)); err != nil {
		return err
	}
	startup := "// Managed by Clutchbase.\nexec server.cfg\n"
	if in.GSLT != "" {
		startup += "sv_setsteamaccount \"" + in.GSLT + "\"\n"
	}
	startup += "map " + mapName + "\n"
	return atomicWrite(filepath.Join(cfgDir, "clutchbase.cfg"), []byte(startup))
}
func unitQuote(value string) string {
	value = strings.NewReplacer("%", "%%", "\\", "\\\\", "\"", "\\\"").Replace(value)
	return "\"" + value + "\""
}
func (m *Manager) unitMarker(name string) string {
	dir, _ := m.Dir(name)
	return "# Managed by Clutchbase.\n# Instance: " + dir + "\n"
}
func (m *Manager) checkUnit(name string) error {
	data, err := os.ReadFile(filepath.Join(m.UnitDir, unit(name)))
	if err == nil && !strings.HasPrefix(string(data), m.unitMarker(name)) {
		return errors.New("service name already belongs to another configuration or instance root")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func (m *Manager) writeUnit(in Instance) error {
	if err := os.MkdirAll(m.UnitDir, 0700); err != nil {
		return err
	}
	dir, _ := m.Dir(in.Name)
	serverDir := filepath.Join(dir, "server")
	marker := m.unitMarker(in.Name)
	path := filepath.Join(m.UnitDir, unit(in.Name))
	if err := m.checkUnit(in.Name); err != nil {
		return err
	}
	// WorkingDirectory is a raw path; unlike ExecStart it does not unquote words.
	launch := unitQuote(strings.ReplaceAll(filepath.Join(serverDir, "game", "cs2.sh"), "$", "$$"))
	if len(in.Whitelist) != 0 {
		guard := filepath.Join(dir, "guard")
		info, err := os.Stat(guard)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
			return errors.New("install clutchbase-guard beside the host and agent binaries, then reconfigure the whitelist")
		}
		launch = unitQuote(strings.ReplaceAll(guard, "$", "$$")) + " --access-dir " + unitQuote(strings.ReplaceAll(m.accessDir(in.Name), "$", "$$")) + " -- " + launch
	}
	data := marker + "[Unit]\nDescription=Clutchbase CS2 " + in.Name + "\nAfter=network-online.target\n\n[Service]\nType=exec\nWorkingDirectory=" + strings.ReplaceAll(serverDir, "%", "%%") + "\nExecStart=" + launch + " -dedicated -console -usercon -strictportbind -port " + strconv.Itoa(in.Port) + " -clientport " + strconv.Itoa(in.Port+1) + " +game_type 0 +game_mode 1 +exec clutchbase.cfg\nKillSignal=SIGINT\nTimeoutStopSec=20\nRestart=no\n\n[Install]\nWantedBy=default.target\n"
	return atomicWrite(path, []byte(data))
}
func (m *Manager) Start(ctx context.Context, name, mapName string) error {
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	in, err := m.Load(name)
	if err != nil {
		return err
	}
	dir, _ := m.Dir(name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if !m.installed(name) {
		return errors.New("install the instance before starting it")
	}
	if len(in.Whitelist) != 0 && access.Failure(m.accessDir(name)) {
		return errors.New("runtime whitelist verification failed; stop the instance before retrying")
	}
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state == "running" || state == "starting" {
		return nil
	}
	if state == "stopping" {
		return errors.New("wait for the instance to stop")
	}
	if err = m.writeConfig(in, mapName); err != nil {
		return err
	}
	if err = m.writeUnit(in); err != nil {
		return err
	}
	if err = systemctl(ctx, "daemon-reload"); err != nil {
		return err
	}
	in.Map = mapName
	data, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(dir, "instance.json"), data); err != nil {
		return err
	}
	return systemctl(ctx, "start", unit(name))
}
func (m *Manager) Stop(ctx context.Context, name string) error {
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	if _, err := m.Load(name); err != nil {
		return err
	}
	dir, _ := m.Dir(name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if !m.installed(name) {
		return nil
	}
	state, err := serviceState(ctx, unit(name))
	if err != nil {
		return err
	}
	if state != "stopped" {
		if err := systemctl(ctx, "stop", unit(name)); err != nil {
			return err
		}
	}
	// Inactive units may unload before reset-failed; only failed units need it.
	if state == "error" {
		if err := systemctl(ctx, "reset-failed", unit(name)); err != nil {
			return err
		}
	}
	if err := os.Remove(filepath.Join(m.accessDir(name), "failure.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func (m *Manager) List(ctx context.Context) ([]Info, error) {
	entries, err := os.ReadDir(filepath.Join(m.Root, "instances"))
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	infos := []Info{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		in, err := m.Load(entry.Name())
		if err != nil {
			return nil, err
		}
		infos = append(infos, Info{Name: in.Name, Port: in.Port, Map: in.Map, State: m.Status(ctx, in.Name).State})
	}
	return infos, nil
}
func (m *Manager) Logs(name string, output io.Writer) error {
	dir, err := m.Dir(name)
	if err != nil {
		return err
	}
	file, err := os.Open(filepath.Join(dir, "install.log"))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(output, file)
	return err
}
