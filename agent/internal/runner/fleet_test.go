package runner

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clutchbase/agent/internal/access"
	"clutchbase/agent/internal/manager"
	"clutchbase/internal/protocol"
)

type recordingController struct{ applies int }

func (r *recordingController) Status(context.Context) protocol.Report {
	return protocol.Report{Driver: "managed", State: "stopped"}
}
func (r *recordingController) Apply(context.Context, protocol.Desired) error { r.applies++; return nil }

func TestInvalidResultAckStillReconciles(t *testing.T) {
	m := &manager.Manager{Root: t.TempDir()}
	id := rand.Text()
	controller := &recordingController{}
	f := &Fleet{manager: m, entries: map[string]*fleetEntry{id: {runner: controller, release: func() {}}}}
	ack := &protocol.MatchResult{SetupID: rand.Text(), CT: 13, T: 9}
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: id, ResultAck: ack, Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}}}}
	if err := f.Apply(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if controller.applies != 1 || f.entries[id].lastError == "" {
		t.Fatal("missing acknowledgement must not prevent reconciliation")
	}
}

func TestFleetHoldsPlayerStatsUntilBackendSupportsThem(t *testing.T) {
	m := &manager.Manager{Root: t.TempDir()}
	id := rand.Text()
	controller := &recordingController{}
	f := &Fleet{manager: m, entries: map[string]*fleetEntry{id: {runner: controller, release: func() {}}}}
	dir, _ := m.Dir("s-" + strings.ToLower(id))
	dir = filepath.Join(dir, "server", "game", "csgo", "cfg", "ClutchbaseAccess")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	setup := rand.Text()
	file := map[string]any{"setupId": setup, "ct": -1, "t": -1, "scoreCt": 13, "scoreT": 9, "players": []protocol.ReportedPlayer{{SteamID: "76561198154367261", Kills: 10, Deaths: 5, Assists: 2}}}
	if err := access.Write(filepath.Join(dir, "result-finished-"+setup+".json"), file); err != nil {
		t.Fatal(err)
	}
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: id, Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}}}}
	if err := f.Apply(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if f.Report(context.Background()).Servers[0].MatchResult != nil {
		t.Fatal("sent stats to old backend")
	}
	desired.SupportsResultStats = true
	if err := f.Apply(context.Background(), desired); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(context.Background()).Servers[0].MatchResult; got == nil || len(got.Players) != 1 || got.CT != 13 {
		t.Fatalf("capable backend missed stats: %+v", got)
	}
	f.ConnectionFailed()
	if f.Report(context.Background()).Servers[0].MatchResult != nil {
		t.Fatal("did not reset capability after failure")
	}
}

func TestFleetResultAckDoesNotBlockReconciliation(t *testing.T) {
	fixture, _ := managedFixture(t)
	f, err := NewFleet(fixture.Manager, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx := context.Background()
	id := rand.Text()
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: id, Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}}}}
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	instance, _ := fixture.Manager.Dir("s-" + strings.ToLower(id))
	dir := filepath.Join(instance, "server", "game", "csgo", "cfg", "ClutchbaseAccess")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	result := protocol.MatchResult{SetupID: rand.Text(), CT: 13, T: 9}
	if err := access.Write(filepath.Join(dir, "result-finished-"+result.SetupID+".json"), result); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(ctx).Servers[0].MatchResult; got == nil || !got.Equal(result) {
		t.Fatalf("result not reported: %+v", got)
	}
	wrong := result
	wrong.T = 8
	desired.Servers[0].ResultAck = &wrong
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(ctx).Servers[0]; got.MatchResult == nil || got.Error == "" {
		t.Fatalf("bad ack lost result: %+v", got)
	}
	desired.Servers[0].ResultAck = &result
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(ctx).Servers[0]; got.MatchResult != nil || got.Error != "" {
		t.Fatalf("result did not clear: %+v", got)
	}
}

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

func TestFleetRemovalRetainsFilesAndRecoversAfterRestart(t *testing.T) {
	fixture, _ := managedFixture(t)
	f, err := NewFleet(fixture.Manager, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id := rand.Text()
	name := "s-" + strings.ToLower(id)
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: id, Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}}}}
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if !f.Report(ctx).SupportsRemoval {
		t.Fatal("removal capability missing")
	}
	desired.Servers[0].Remove = true
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(ctx).Servers[0]; got.State != "removed" || got.Error != "" {
		t.Fatalf("removal not acknowledged: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(fixture.Manager.Root, "retired", name, "instance.json")); err != nil {
		t.Fatal("files were not retained", err)
	}
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal("removal retry failed", err)
	}
	f.Close()
	f, err = NewFleet(fixture.Manager, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if got := f.Report(ctx).Servers[0]; got.State != "removed" {
		t.Fatalf("restart lost removal progress: %+v", got)
	}
	if err := f.Apply(ctx, protocol.MachineDesired{}); err != nil {
		t.Fatal(err)
	}
	if len(f.Report(ctx).Servers) != 0 {
		t.Fatal("acknowledged controller remains")
	}
	m := *fixture.Manager
	release, err := m.AcquireAgent(name)
	if err != nil {
		t.Fatal("removed controller lease retained", err)
	}
	release()
}

func TestFleetRosterAcknowledgement(t *testing.T) {
	f, err := NewFleet(&manager.Manager{Root: t.TempDir()}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ids := []string{"76561198154367261"}
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: rand.Text(), Desired: protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015, Whitelist: &ids}}}}
	ctx := context.Background()
	if err := f.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	report := f.Report(ctx)
	if !report.SupportsWhitelist || report.Servers[0].WhitelistHash != protocol.WhitelistHash(ids) {
		t.Fatal("missing applied roster acknowledgement")
	}
	invalid := []string{"bad"}
	desired.Servers[0].Whitelist = &invalid
	desired.Servers[0].State = "running"
	if err := f.Apply(ctx, desired); err == nil {
		t.Fatal("invalid roster accepted")
	}
	if f.Report(ctx).Servers[0].State != "stopped" {
		t.Fatal("invalid roster partially applied")
	}
}

func TestManagedRosterFailureBlocksStart(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	desired := protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	ids := []string{"76561198154367261"}
	desired.Whitelist = &ids
	desired.State = "running"
	if err := r.Apply(ctx, desired); err == nil {
		t.Fatal("started without applying roster")
	}
	if r.Status(ctx).WhitelistHash != "" {
		t.Fatal("failed roster acknowledged")
	}
	if _, err := os.Stat(filepath.Join(root, "begun")); !os.IsNotExist(err) {
		t.Fatal("roster failure launched provisioning")
	}
	disabled := []string{}
	desired.State = "stopped"
	desired.Whitelist = &disabled
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).WhitelistHash != protocol.WhitelistHash(disabled) {
		t.Fatal("disable not acknowledged")
	}
}

func TestManagedRosterAppliedAndReconciledBeforeLaunch(t *testing.T) {
	r, root := managedFixture(t)
	ctx := context.Background()
	desired := protocol.Desired{State: "stopped", Map: "de_nuke", Port: 27015}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Manager.Install(ctx, r.Name, false); err != nil {
		t.Fatal(err)
	}
	r.Manager.Guard = filepath.Join(root, "guard")
	if err := os.WriteFile(r.Manager.Guard, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	dir, _ := r.Manager.Dir(r.Name)
	for _, file := range []string{
		"addons/metamod/bin/linuxsteamrt64/libserver.so",
		"addons/metamod/bin/linuxsteamrt64/metamod.2.cs2.so",
		"addons/metamod/counterstrikesharp.vdf",
		"addons/counterstrikesharp/bin/linuxsteamrt64/counterstrikesharp.so",
		"addons/counterstrikesharp/plugins/MatchZy/MatchZy.dll",
		"addons/counterstrikesharp/plugins/ClutchbaseAccess/ClutchbaseAccess.dll",
		"gameinfo.gi",
	} {
		path := filepath.Join(dir, "server/game/csgo", file)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("Game csgo/addons/metamod\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{"76561198154367261"}
	desired.Whitelist = &ids
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if r.Status(ctx).WhitelistHash != protocol.WhitelistHash(ids) {
		t.Fatal("roster not acknowledged")
	}
	// Persisted intent alone is insufficient: repair a partial write before ack.
	rosterFile := filepath.Join(dir, "server/game/csgo/cfg/MatchZy/whitelist.cfg")
	if err := os.Remove(rosterFile); err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(rosterFile)
	if err != nil || string(data) != ids[0]+"\n" {
		t.Fatal("partial roster not repaired", err)
	}
	// A restart reconstructs its acknowledgement from local files and desired state.
	restarted := &Managed{Manager: r.Manager, Name: r.Name}
	if err := restarted.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	if restarted.Status(ctx).WhitelistHash != protocol.WhitelistHash(ids) {
		t.Fatal("restart lost acknowledgement")
	}
	desired.State = "running"
	if err := restarted.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "server/game/csgo/cfg/ClutchbaseAccess/request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Apply(ctx, desired); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "server/game/csgo/cfg/ClutchbaseAccess/request.json"))
	if string(before) != string(after) {
		t.Fatal("reconciliation invalidated launch nonce")
	}
	// Even instructions bypassing the API cannot replace the live roster.
	next := []string{"76561197960265729"}
	desired.Whitelist = &next
	if err := restarted.Apply(ctx, desired); err == nil {
		t.Fatal("changed live roster")
	}
	if restarted.Status(ctx).WhitelistHash != "" {
		t.Fatal("failed live edit acknowledged")
	}
}

func TestRestartCapabilityIsManagedOnly(t *testing.T) {
	ctx := context.Background()
	m := &manager.Manager{Root: t.TempDir()}
	managed := &Fleet{manager: m, entries: map[string]*fleetEntry{}}
	if !managed.Report(ctx).SupportsRestart {
		t.Fatal("managed agent did not advertise restart")
	}
	simulated := &Fleet{manager: m, simulate: true, entries: map[string]*fleetEntry{}}
	if simulated.Report(ctx).SupportsRestart {
		t.Fatal("simulation advertised restart")
	}
	id := rand.Text()
	desired := protocol.MachineDesired{Servers: []protocol.ServerDesired{{ID: id, Desired: protocol.Desired{State: "stopped", Map: "de_mirage", Port: 27015, Restart: &protocol.RestartCommand{ID: rand.Text(), Phase: "stop"}}}}}
	if err := simulated.Apply(ctx, desired); err == nil {
		t.Fatal("simulation accepted restart")
	}
}
