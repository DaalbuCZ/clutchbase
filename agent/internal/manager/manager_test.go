package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CB_TEST_STATE", filepath.Join(dir, "states"))
	t.Setenv("CB_TEST_ARGS", filepath.Join(dir, "steam-args"))
	if err := os.Mkdir(filepath.Join(dir, "states"), 0700); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		"systemctl": `#!/bin/sh
for arg in "$@"; do last="$arg"; done
case " $* " in
  *" show "*) if [ "$CB_TEST_BAD_UNIT" = 1 ]; then echo bad-setting; else echo loaded; fi ;;
  *" is-active "*)
    if [ -f "$CB_TEST_STATE/$last" ]; then cat "$CB_TEST_STATE/$last"; else echo inactive; fi ;;
  *" start "*) echo active > "$CB_TEST_STATE/$last" ;;
  *" stop "*) echo inactive > "$CB_TEST_STATE/$last" ;;
esac
`,
		"steamcmd": `#!/bin/sh
printf '%s\n' "$@" > "$CB_TEST_ARGS"
if [ "$CB_TEST_FAIL" = 1 ]; then exit 8; fi
if [ "$CB_TEST_SLOW" = 1 ]; then sleep 60; fi
mkdir -p "$2/game/csgo/cfg"
printf '#!/bin/sh\nexit 0\n' > "$2/game/cs2.sh"
chmod +x "$2/game/cs2.sh"
if [ "$CB_TEST_PARTIAL" = 1 ]; then exit 0; fi
mkdir -p "$2/game/bin/linuxsteamrt64"
cp "$2/game/cs2.sh" "$2/game/bin/linuxsteamrt64/cs2"
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return &Manager{Root: filepath.Join(dir, "data with spaces%"), UnitDir: filepath.Join(dir, "units"), SteamCMD: filepath.Join(bin, "steamcmd")}
}
func create(t *testing.T, m *Manager, name string, port int) Instance {
	t.Helper()
	in, err := m.Create(Instance{Name: name, Port: port, Map: "de_mirage", GSLT: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return in
}
func TestCreateIsolationAndSecrets(t *testing.T) {
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	if len(in.Password) < 26 {
		t.Fatal("missing random join password")
	}
	for _, bad := range []Instance{{Name: "../escape", Port: 27025, Map: "de_mirage"}, {Name: "bad", Port: 65535, Map: "de_mirage"}, {Name: "bad", Port: 27025, Map: "de_mirage;quit"}, {Name: "bad", Port: 27025, Map: "de_mirage", Password: "x\nquit"}, {Name: "bad", Port: 27025, Map: "de_mirage", GSLT: "invalid"}, {Name: "overlap", Port: 27017, Map: "de_mirage"}, {Name: "reused-token", Port: 27025, Map: "de_mirage", GSLT: in.GSLT}} {
		if _, err := m.Create(bad); err == nil {
			t.Fatalf("accepted invalid instance %s", bad.Name)
		}
	}
	if _, err := m.Create(Instance{Name: "second", Port: 27025, Map: "de_nuke"}); err != nil {
		t.Fatal(err)
	}
	infos, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 {
		t.Fatal("missing isolated instances")
	}
	data, _ := json.Marshal(infos)
	if strings.Contains(string(data), in.Password) || strings.Contains(string(data), in.GSLT) {
		t.Fatal("list leaked credentials")
	}
	dir, _ := m.Dir(in.Name)
	stat, err := os.Stat(filepath.Join(dir, "instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0600 {
		t.Fatal("instance credentials are not private")
	}
}
func TestInstallStartStopAndUpdate(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	ctx := context.Background()
	if m.Status(ctx, "friends").State != "uninstalled" {
		t.Fatal("expected uninstalled")
	}
	if err := m.Install(ctx, "friends", false); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(os.Getenv("CB_TEST_ARGS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "+login\nanonymous\n+app_update\n730\nvalidate\n+quit") || !strings.HasPrefix(string(args), "+force_install_dir\n") {
		t.Fatalf("wrong SteamCMD arguments: %s", args)
	}
	service, err := os.ReadFile(filepath.Join(m.UnitDir, unit("friends")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(service), "spaces%%") || !strings.Contains(string(service), "game/cs2.sh\" -dedicated") || !strings.Contains(string(service), "-port 27015 -clientport 27016") {
		t.Fatalf("incorrect unit: %s", service)
	}
	dir, _ := m.Dir("friends")
	workingDirectory := "WorkingDirectory=" + strings.ReplaceAll(filepath.Join(dir, "server"), "%", "%%") + "\n"
	if !strings.Contains(string(service), workingDirectory) {
		t.Fatal("WorkingDirectory must be an unquoted absolute path, including spaces")
	}
	inConfig, err := m.Load("friends")
	if err != nil {
		t.Fatal(err)
	}
	serverConfig, err := os.ReadFile(filepath.Join(dir, "server", "game", "csgo", "cfg", "server.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serverConfig), "sv_password \""+inConfig.Password+"\"") || strings.Contains(string(serverConfig), "\nmap ") {
		t.Fatal("private settings must reapply after map load without recursively loading the map")
	}
	startupConfig, err := os.ReadFile(filepath.Join(dir, "server", "game", "csgo", "cfg", "clutchbase.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(startupConfig), "exec server.cfg\n") || !strings.HasSuffix(string(startupConfig), "map de_mirage\n") {
		t.Fatal("startup config must load settings before the initial map")
	}
	if err := m.Start(ctx, "friends", "de_nuke"); err != nil {
		t.Fatal(err)
	}
	if m.Status(ctx, "friends").State != "running" {
		t.Fatal("instance did not start")
	}
	if err := m.Install(ctx, "friends", true); err == nil {
		t.Fatal("updated active instance")
	}
	if err := m.Install(ctx, "friends", false); err != nil {
		t.Fatal("idempotent install should not change running files:", err)
	}
	if err := m.Stop(ctx, "friends"); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(ctx, "friends", true); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(os.Getenv("CB_TEST_ARGS"))
	if strings.Contains(string(args), "validate") {
		t.Fatal("routine update unexpectedly validates default files")
	}
	in, err := m.Load("friends")
	if err != nil {
		t.Fatal(err)
	}
	if in.Map != "de_nuke" {
		t.Fatal("managed startup map was not persisted")
	}
}
func TestFailedAndCancelledInstallIsNotReady(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	ctx := context.Background()
	t.Setenv("CB_TEST_FAIL", "1")
	if err := m.Install(ctx, "friends", false); err == nil {
		t.Fatal("failed install succeeded")
	}
	if m.installed("friends") {
		t.Fatal("failed installation marked ready")
	}
	t.Setenv("CB_TEST_FAIL", "")
	t.Setenv("CB_TEST_SLOW", "1")
	job, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if err := m.Install(job, "friends", false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	if m.installed("friends") {
		t.Fatal("cancelled install marked ready")
	}
	t.Setenv("CB_TEST_SLOW", "")
	if err := m.Install(ctx, "friends", false); err != nil {
		t.Fatal("could not resume partial installation:", err)
	}
}
func TestAgentExcludesOtherControllers(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	unlinked := *m
	release, err := m.AcquireAgent("friends")
	if err != nil {
		t.Fatal(err)
	}
	if err := unlinked.Stop(context.Background(), "friends"); err == nil {
		t.Fatal("CLI mutated an agent-controlled instance")
	}
	if _, err := unlinked.AcquireAgent("friends"); err == nil {
		t.Fatal("two agents acquired the same instance")
	}
	if err := m.Stop(context.Background(), "friends"); err != nil {
		t.Fatal("owning agent was blocked:", err)
	}
	release()
	if err := unlinked.Stop(context.Background(), "friends"); err != nil {
		t.Fatal("lease not released:", err)
	}
}

func TestDoesNotOverwriteOtherServices(t *testing.T) {
	m := fixture(t)
	if err := os.MkdirAll(m.UnitDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.UnitDir, unit("friends"))
	for _, content := range []string{"[Service]\nExecStart=/bin/true\n", "# Managed by Clutchbase.\n# Instance: /some/other/root\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Create(Instance{Name: "friends", Port: 27015, Map: "de_mirage"}); err == nil {
			t.Fatal("reused a service belonging to a different owner")
		}
		data, _ := os.ReadFile(path)
		if string(data) != content {
			t.Fatal("modified another service")
		}
	}
}

func TestWaitingForSteamCMDIsCancellable(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	release, err := lock(filepath.Join(m.Root, "steamcmd.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := m.Install(ctx, "friends", false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued install was not cancelled: %v", err)
	}
	if _, err := os.Stat(os.Getenv("CB_TEST_ARGS")); err == nil {
		t.Fatal("SteamCMD ran while another install owned the lock")
	}
	if m.installed("friends") {
		t.Fatal("queued installation was marked ready")
	}
}

func TestPartialFilesAreNotReady(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	t.Setenv("CB_TEST_PARTIAL", "1")
	if err := m.Install(context.Background(), "friends", false); err == nil {
		t.Fatal("partial files were treated as installed")
	}
	if m.installed("friends") {
		t.Fatal("partial installation was marked ready")
	}
	t.Setenv("CB_TEST_PARTIAL", "")
	if err := m.Install(context.Background(), "friends", false); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidSystemdUnitIsNotReady(t *testing.T) {
	m := fixture(t)
	create(t, m, "friends", 27015)
	t.Setenv("CB_TEST_BAD_UNIT", "1")
	if err := m.Install(context.Background(), "friends", false); err == nil {
		t.Fatal("an invalid systemd unit was treated as ready")
	}
	if m.installed("friends") {
		t.Fatal("invalid unit received an installed marker")
	}
}
