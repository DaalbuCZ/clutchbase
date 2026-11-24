package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRetirePreservesFilesAndFreesPort(t *testing.T) {
	m := fixture(t)
	ctx := context.Background()
	in := create(t, m, "friends", 27015)
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	dir, _ := m.Dir(in.Name)
	original, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx, in.Name, in.Map); err != nil {
		t.Fatal(err)
	}
	if err := m.Retire(ctx, in.Name); err == nil {
		t.Fatal("retired a live service")
	}
	if err := m.Stop(ctx, in.Name); err != nil {
		t.Fatal(err)
	}
	if err := m.Retire(ctx, in.Name); err != nil {
		t.Fatal(err)
	}
	retained := filepath.Join(m.Root, "retired", in.Name)
	data, err := os.ReadFile(filepath.Join(retained, "instance.json"))
	if err != nil || string(data) != string(original) {
		t.Fatalf("private configuration not retained: %v", err)
	}
	if !gameFilesPresent(filepath.Join(retained, "server")) {
		t.Fatal("game files not retained")
	}
	if _, err := os.Stat(filepath.Join(m.UnitDir, unit(in.Name))); !os.IsNotExist(err) {
		t.Fatal("managed unit remains")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("instance remains active locally")
	}
	info, err := os.Stat(filepath.Dir(retained))
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatal("retained root is not private")
	}
	if err := m.Retire(ctx, in.Name); err != nil {
		t.Fatalf("retry not idempotent: %v", err)
	}
	if _, err := m.Create(Instance{Name: "replacement", Port: 27015, Map: in.Map}); err != nil {
		t.Fatalf("retired port not reusable: %v", err)
	}
}

func TestRetireHonorsOwnershipAndLocks(t *testing.T) {
	m := fixture(t)
	ctx := context.Background()
	in := create(t, m, "friends", 27015)
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	other := *m
	release, err := m.AcquireAgent(in.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Retire(ctx, in.Name); err == nil {
		t.Fatal("bypassed controller lease")
	}
	release()
	m.agentName = ""
	dir, _ := m.Dir(in.Name)
	unlock, err := lock(filepath.Join(dir, "operation.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Retire(ctx, in.Name); err == nil {
		t.Fatal("bypassed install lock")
	}
	unlock()
	unitPath := filepath.Join(m.UnitDir, unit(in.Name))
	if err := os.WriteFile(unitPath, []byte("# another manager owns this service\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Retire(ctx, in.Name); err == nil {
		t.Fatal("removed another manager's unit")
	}
	if _, err := os.Stat(filepath.Join(dir, "instance.json")); err != nil {
		t.Fatal("moved files despite failed ownership check")
	}
}

func TestRetireNeverOverwritesRetainedInstallation(t *testing.T) {
	m := fixture(t)
	ctx := context.Background()
	in := create(t, m, "friends", 27015)
	retained := filepath.Join(m.Root, "retired", in.Name)
	if err := os.MkdirAll(retained, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(retained, "keep"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Retire(ctx, in.Name); err == nil {
		t.Fatal("overwrote retained files")
	}
	data, err := os.ReadFile(filepath.Join(retained, "keep"))
	if err != nil || string(data) != "preserve" {
		t.Fatal("retained data modified")
	}
	if err := m.Retire(ctx, "../escape"); err == nil {
		t.Fatal("accepted path as instance name")
	}
}
