package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"clutchbase/internal/protocol"
)

// An existing-service runner must also prove joinability through the game
// query, using the registered game port learned from desired state.
func TestSystemdRunnerReportsGameQueryReadiness(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	stateFile := filepath.Join(dir, "state")
	scripts := map[string]string{
		"systemctl": `#!/bin/sh
case " $* " in
 *" is-active "*) if [ -f "$CB_UNIT_STATE" ]; then cat "$CB_UNIT_STATE"; else echo inactive; fi ;;
 *" start "*) echo active > "$CB_UNIT_STATE" ;;
 *" stop "*) echo inactive > "$CB_UNIT_STATE" ;;
esac
`,
		"journalctl": `#!/bin/sh
echo "CS2 console line"
`,
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CB_UNIT_STATE", stateFile)
	if err := os.Mkdir(filepath.Join(dir, "cfg"), 0700); err != nil {
		t.Fatal(err)
	}
	r := &Runner{Unit: "clutchbase-existing.service", ConfigPath: filepath.Join(dir, "cfg", "clutchbase.cfg"), Port: 27015}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := r.Apply(ctx, protocol.Desired{State: "running", Map: "de_mirage", Port: 27015}); err != nil {
		t.Fatal(err)
	}
	if got := r.Status(ctx); got.State != "starting" || got.LogTail != "CS2 console line" {
		t.Fatalf("active service without query answer must stay starting: %+v", got)
	}
	serveA2S(t, 27015, 1, 12)
	if got := r.Status(ctx); got.State != "running" || got.Players == nil || *got.Players != 1 || got.MaxPlayers == nil || *got.MaxPlayers != 12 {
		t.Fatalf("game query did not prove readiness: %+v", got)
	}
	if err := os.WriteFile(stateFile, []byte("inactive"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := r.Status(ctx); got.State != "stopped" {
		t.Fatalf("inactive service must be stopped: %+v", got)
	}
}

func TestSimulationAndRejectedCommands(t *testing.T) {
	r := &Runner{Simulate: true}
	ctx := context.Background()
	if r.Status(ctx).State != "stopped" {
		t.Fatal("simulation must start stopped")
	}
	if err := r.Apply(ctx, protocol.Desired{State: "running", Map: "de_mirage"}); err != nil {
		t.Fatal(err)
	}
	if got := r.Status(ctx); got.State != "running" || got.Driver != "simulated" {
		t.Fatalf("unexpected report: %+v", got)
	}
	for _, desired := range []protocol.Desired{{State: "restart", Map: "de_mirage"}, {State: "running", Map: "de_mirage; quit"}} {
		if err := r.Apply(ctx, desired); err == nil {
			t.Fatalf("accepted invalid command: %+v", desired)
		}
	}
	if err := r.Apply(ctx, protocol.Desired{State: "stopped", Map: "de_mirage"}); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).State != "stopped" {
		t.Fatal("stop did not converge")
	}
}
func TestConfigIsValidatedAndReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clutchbase.cfg")
	if err := writeConfig(path, "de_nuke"); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(path, "de_nuke\nquit"); err == nil {
		t.Fatal("accepted injected config")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "// Managed by Clutchbase.\nmap de_nuke\n" {
		t.Fatalf("unexpected config: %q", data)
	}
	if err := (&Runner{Unit: "--help", ConfigPath: path}).Validate(); err == nil {
		t.Fatal("accepted option as unit")
	}
}
