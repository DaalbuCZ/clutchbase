package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"clutchbase/internal/protocol"
)

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
