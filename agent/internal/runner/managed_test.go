package runner

import (
	"context"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

func managedFixture(t *testing.T) (*Managed, string) {
	t.Helper()
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CB_JOB_DIR", root)
	scripts := map[string]string{
		"systemctl": `#!/bin/sh
case " $* " in
 *" show "*) echo loaded ;;
 *" is-active "*) if [ -f "$CB_JOB_DIR/running" ]; then echo active; else echo inactive; fi ;;
 *" start "*) echo start >> "$CB_JOB_DIR/actions"; touch "$CB_JOB_DIR/running" ;;
 *" stop "*) rm -f "$CB_JOB_DIR/running" ;;
esac
`,
		"steamcmd": `#!/bin/sh
touch "$CB_JOB_DIR/begun"
while [ ! -f "$CB_JOB_DIR/release" ]; do sleep 0.01; done
mkdir -p "$2/game/csgo/cfg"
printf '#!/bin/sh\nexit 0\n' > "$2/game/cs2.sh"
chmod +x "$2/game/cs2.sh"
mkdir -p "$2/game/bin/linuxsteamrt64"
cp "$2/game/cs2.sh" "$2/game/bin/linuxsteamrt64/cs2"
`,
		"journalctl": `#!/bin/sh
echo "CS2 console line"
echo 'sv_password "hunter2"'
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	m := &manager.Manager{Root: filepath.Join(root, "data"), UnitDir: filepath.Join(root, "units"), SteamCMD: filepath.Join(bin, "steamcmd")}
	release, err := m.AcquireAgent("friends")
	if err != nil {
		t.Fatal(err)
	}
	r := &Managed{Manager: m, Name: "friends"}
	t.Cleanup(func() { r.Close(); release() })
	return r, root
}

func TestManagedGameUpdateRunsOnlyForStoppedServer(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	in, err := r.Manager.Create(manager.Instance{Name: r.Name, Port: 27015, Map: "de_mirage"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Manager.Install(ctx, r.Name, false); err != nil {
		t.Fatal(err)
	}
	command := &protocol.GameUpdate{ID: rand.Text()}
	if err := r.Apply(ctx, protocol.Desired{State: "running", Map: in.Map, Port: in.Port, GameUpdate: command}); err == nil {
		t.Fatal("accepted an update while the desired state was running")
	}
	if err := r.Apply(ctx, protocol.Desired{State: "stopped", Map: in.Map, Port: in.Port, GameUpdate: command}); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return r.Status(ctx).GameUpdateStatus == "complete" })
	if got := r.Status(ctx); got.GameUpdateID != command.ID || got.GameUpdateError != "" {
		t.Fatalf("unexpected game update result: %+v", got)
	}
}
func eventually(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition did not converge")
}

// serveA2S answers loopback game queries with a ready-server reply until the
// test ends, standing in for CS2 accepting queries on the instance port.
func serveA2S(t *testing.T, port int, players, max byte) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < 5 {
				continue
			}
			reply := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49, 24}
			reply = append(reply, "clutchbase\x00de_mirage\x00csgo\x00CS2\x00"...)
			reply = append(reply, 0, 0, players, max, 0, 'd', 'l', 0, 0)
			conn.WriteToUDP(reply, from)
		}
	}()
}
func TestManagedInstallKeepsStatusLiveAndStartsOnReconciliation(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	desired := protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).State != "uninstalled" {
		t.Fatal("instance should await a start request")
	}
	desired.State = "running"
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).State != "installing" {
		t.Fatal("installation was not observable")
	}
	eventually(t, func() bool { _, err := os.Stat(filepath.Join(root, "begun")); return err == nil })
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return r.Status(ctx).State != "installing" })
	if r.Status(ctx).State != "stopped" {
		t.Fatal("installer should not autonomously start the server")
	}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	serveA2S(t, 27015, 3, 12)
	got := r.Status(ctx)
	// The fake engine reports Mirage even though Nuke was requested at startup.
	if got.State != "running" || got.ActiveMap != "de_mirage" || got.Players == nil || *got.Players != 3 || got.MaxPlayers == nil || *got.MaxPlayers != 12 {
		t.Fatalf("wrong running state: %+v", got)
	}
	if got.LogTail == "" || strings.Contains(got.LogTail, "hunter2") {
		t.Fatalf("log tail missing or not redacted: %q", got.LogTail)
	}
	desired.State = "stopped"
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).State != "stopped" {
		t.Fatal("stop did not converge")
	}
}
func TestManagedCancellationAndPortMismatch(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	desired := protocol.Desired{State: "running", Map: "de_mirage", Port: 27015}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { _, err := os.Stat(filepath.Join(root, "begun")); return err == nil })
	desired.State = "stopped"
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return r.Status(ctx).State != "installing" })
	if got := r.Status(ctx); got.State != "uninstalled" || got.Error != "" {
		t.Fatalf("partial installation not cancelled cleanly: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "running")); err == nil {
		t.Fatal("cancelled job started a service")
	}
	desired.Port = 27025
	if err := r.Apply(ctx, desired); err == nil {
		t.Fatal("accepted a mismatch between registration and local game port")
	}
}

func TestManagedRestartRequiresMatchingStopAndExplicitStart(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	in, err := r.Manager.Create(manager.Instance{Name: r.Name, Port: 27015, Map: "de_mirage"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Manager.Install(ctx, r.Name, false); err != nil {
		t.Fatal(err)
	}
	id := rand.Text()
	whitelist := []string{}
	preset := ""
	desired := protocol.Desired{State: "stopped", Map: in.Map, Port: in.Port, Whitelist: &whitelist, Preset: &preset, Restart: &protocol.RestartCommand{ID: id, Phase: "stop"}}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	report := r.Status(ctx)
	if report.RestartID != id || report.RestartPhase != "stopped" {
		t.Fatalf("stop not confirmed: %+v", report)
	}
	// A new runner withholds the acknowledgement until configuration is restored.
	recovered := &Managed{Manager: r.Manager, Name: r.Name}
	if report = recovered.Status(ctx); report.RestartID != "" || report.RestartPhase != "" {
		t.Fatalf("reported stopped before recovery: %+v", report)
	}
	if err := recovered.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if report = recovered.Status(ctx); report.RestartID != id || report.RestartPhase != "stopped" || report.WhitelistHash != protocol.WhitelistHash(whitelist) || report.AppliedPreset == nil || *report.AppliedPreset != preset {
		t.Fatalf("failed to restore acknowledgement: %+v", report)
	}
	if err := recovered.Apply(ctx, protocol.Desired{State: "running", Map: in.Map, Port: in.Port}); err == nil {
		t.Fatal("ordinary desired state bypassed pending restart")
	}
	wrong := protocol.Desired{State: "running", Map: in.Map, Port: in.Port, Restart: &protocol.RestartCommand{ID: rand.Text(), Phase: "start"}}
	if err := recovered.Apply(ctx, wrong); err == nil {
		t.Fatal("another request authorized restart")
	}
	desired.Restart.Phase = "cancel"
	if err := recovered.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	late := protocol.Desired{State: "running", Map: in.Map, Port: in.Port, Restart: &protocol.RestartCommand{ID: id, Phase: "start"}}
	if err := recovered.Apply(ctx, late); err == nil {
		t.Fatal("cancelled restart started")
	}
}

func TestManagedRestartCancellationBeforeDeliveryIsDurable(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	in, err := r.Manager.Create(manager.Instance{Name: r.Name, Port: 27015, Map: "de_mirage"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Manager.Install(ctx, r.Name, false); err != nil {
		t.Fatal(err)
	}
	id := rand.Text()
	desired := protocol.Desired{State: "stopped", Map: in.Map, Port: in.Port, Restart: &protocol.RestartCommand{ID: id, Phase: "cancel"}}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatalf("repeated cancellation: %v", err)
	}
	recovered := &Managed{Manager: r.Manager, Name: r.Name}
	desired.Restart.Phase = "stop"
	if err := recovered.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	report := recovered.Status(ctx)
	if report.RestartID != id || report.RestartPhase != "cancelled" {
		t.Fatalf("delayed stop changed cancellation: %+v", report)
	}
	desired.State = "running"
	desired.Restart.Phase = "start"
	if err := recovered.Apply(ctx, desired); err == nil {
		t.Fatal("cancelled restart started")
	}
	next := rand.Text()
	desired.State = "stopped"
	desired.Restart = &protocol.RestartCommand{ID: next, Phase: "stop"}
	if err := recovered.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if _, _, err := recovered.Manager.RecordRestartFence(ctx, recovered.Name, next, "start_authorized"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "running"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	desired.Restart.Phase = "cancel"
	if err := recovered.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "running")); !os.IsNotExist(err) {
		t.Fatalf("cancellation did not stop server: %v", err)
	}
}

func TestManagedRestartStartIsFencedAndNotRepeated(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	in, err := r.Manager.Create(manager.Instance{Name: r.Name, Port: 27015, Map: "de_mirage"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.Manager.Install(ctx, r.Name, false); err != nil {
		t.Fatal(err)
	}
	id := rand.Text()
	desired := protocol.Desired{State: "stopped", Map: in.Map, Port: in.Port, Restart: &protocol.RestartCommand{ID: id, Phase: "stop"}}
	if err = r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	desired.State = "running"
	desired.Restart.Phase = "start"
	if err = r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "actions"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "start") != 1 {
		t.Fatalf("start repeated: %s", data)
	}
	if report := r.Status(ctx); report.RestartID != id || report.RestartPhase != "starting" {
		t.Fatalf("unexpected acknowledgement: %+v", report)
	}
}
