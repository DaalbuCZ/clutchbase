package runner

import (
	"context"
	"os"
	"path/filepath"
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
 *" start "*) touch "$CB_JOB_DIR/running" ;;
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
	if got := r.Status(ctx); got.State != "running" || got.ActiveMap != "de_nuke" {
		t.Fatalf("wrong running state: %+v", got)
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
