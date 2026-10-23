package runner

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

func TestFleetIndependentServersAndValidation(t *testing.T) {
	m := &manager.Manager{Root: t.TempDir()}
	f, err := NewFleet(m, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	other := &manager.Manager{Root: m.Root}
	if duplicate, err := NewFleet(other, true); err == nil {
		duplicate.Close()
		t.Fatal("duplicate fleet acquired controller")
	}
	ctx := context.Background()
	a, b := rand.Text(), rand.Text()
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{
		{ID: a, Desired: protocol.Desired{State: "running", Map: "de_nuke", Port: 27015}},
		{ID: b, Desired: protocol.Desired{State: "stopped", Map: "de_mirage", Port: 27025}},
	}}
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	reports := func() map[string]protocol.Report {
		out := map[string]protocol.Report{}
		for _, s := range f.Report(ctx).Servers {
			out[s.ID] = s.Report
		}
		return out
	}
	got := reports()
	if got[a].State != "running" || got[a].ActiveMap != "de_nuke" || got[b].State != "stopped" || got[b].Driver != "simulated" {
		t.Fatalf("crossed server state: %+v", got)
	}
	desired.Servers[0].State = "stopped"
	desired.Servers[1].State = "running"
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	got = reports()
	if got[a].State != "stopped" || got[b].State != "running" {
		t.Fatal("sibling command not isolated")
	}
	desired.Servers[0].State = "running"
	desired.Servers[1].ID = "../../unsafe"
	if err = f.Apply(ctx, desired); err == nil {
		t.Fatal("accepted host path as ID")
	}
	if reports()[a].State != "stopped" {
		t.Fatal("partially applied invalid instructions")
	}
	desired.Servers[1].ID = a
	if err = f.Apply(ctx, desired); err == nil {
		t.Fatal("accepted duplicate instance")
	}
	if err = f.Apply(ctx, protocol.MachineDesired{}); err == nil {
		t.Fatal("silently discarded existing controllers")
	}
}

func TestFleetManagedInstallFailureDoesNotBlockSibling(t *testing.T) {
	fixture, root := managedFixture(t)
	f, err := NewFleet(fixture.Manager, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx := context.Background()
	a, b := rand.Text(), rand.Text()
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{
		{ID: a, Desired: protocol.Desired{State: "running", Map: "de_nuke", Port: 27015}},
		{ID: b, Desired: protocol.Desired{State: "stopped", Map: "de_mirage", Port: 27015}},
	}}
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { _, err := os.Stat(filepath.Join(root, "begun")); return err == nil })
	reports := map[string]protocol.Report{}
	for _, s := range f.Report(ctx).Servers {
		reports[s.ID] = s.Report
	}
	if reports[a].State != "installing" || reports[b].Error == "" {
		t.Fatalf("failure was not isolated: %+v", reports)
	}
	// Each runner retains its own lease, even though the machine has one heartbeat.
	independent := &manager.Manager{Root: fixture.Manager.Root}
	if release, err := independent.AcquireAgent("s-" + strings.ToLower(a)); err == nil {
		release()
		t.Fatal("fleet instance lease lost")
	}
	desired.Servers[0].State = "stopped"
	desired.Servers[1].Port = 27025
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		for _, s := range f.Report(ctx).Servers {
			if s.ID == a {
				return s.State != "installing"
			}
		}
		return false
	})
	for _, s := range f.Report(ctx).Servers {
		if s.Error != "" || s.State != "uninstalled" {
			t.Fatalf("did not recover: %+v", s)
		}
	}
}

func TestFleetRetriesContendedInstanceWithoutBlockingSibling(t *testing.T) {
	fixture, _ := managedFixture(t)
	a, b := rand.Text(), rand.Text()
	other := &manager.Manager{Root: fixture.Manager.Root}
	release, err := other.AcquireAgent("s-" + strings.ToLower(a))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	f, err := NewFleet(fixture.Manager, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{
		{ID: a, Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}},
		{ID: b, Desired: protocol.Desired{State: "stopped", Map: "de_mirage", Port: 27025}},
	}}
	ctx := context.Background()
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	for _, s := range f.Report(ctx).Servers {
		if s.ID == a && s.Error == "" {
			t.Fatal("controller contention not reported")
		}
		if s.ID == b && (s.Error != "" || s.State != "uninstalled") {
			t.Fatalf("sibling blocked: %+v", s)
		}
	}
	release()
	release = nil
	if err = f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	for _, s := range f.Report(ctx).Servers {
		if s.Error != "" || s.State != "uninstalled" {
			t.Fatalf("contention did not recover: %+v", s)
		}
	}
}
