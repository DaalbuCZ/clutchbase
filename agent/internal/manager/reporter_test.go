package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReporterUpdateRequiresStoppedServerAndExactBundle(t *testing.T) {
	ctx := context.Background()
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	addPluginFixture(t, m, in.Name)
	presetFixture(t, m)
	if err := m.SetPreset(ctx, in.Name, "match"); err != nil {
		t.Fatal(err)
	}
	installedPath := filepath.Join(m.presetPluginDir(in.Name), "ClutchbaseAccess.dll")
	before, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(m.PresetPlugin, "ClutchbaseAccess.dll"), []byte("new staged reporter"), 0600); err != nil {
		t.Fatal(err)
	}
	source, installed := m.reporterHashes(in.Name)
	if source == "" || installed == "" || source == installed {
		t.Fatal("reporter fingerprints did not distinguish staged and installed files")
	}
	if err = os.WriteFile(filepath.Join(os.Getenv("CB_TEST_STATE"), unit(in.Name)), []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.UpdateReporter(ctx, in.Name, source); err == nil {
		t.Fatal("updated a running server")
	}
	if current, err := os.ReadFile(installedPath); err != nil || string(current) != string(before) {
		t.Fatal("running server reporter changed", err)
	}
	if err = os.WriteFile(filepath.Join(os.Getenv("CB_TEST_STATE"), unit(in.Name)), []byte("inactive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.UpdateReporter(ctx, in.Name, strings.Repeat("0", 64)); err == nil {
		t.Fatal("accepted a different staged fingerprint")
	}
	if err = os.WriteFile(filepath.Join(m.PresetPlugin, "ClutchbaseAccess.dll"), []byte("changed after request"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.UpdateReporter(ctx, in.Name, source); err == nil {
		t.Fatal("accepted a bundle changed after the request")
	}
	if current, err := os.ReadFile(installedPath); err != nil || string(current) != string(before) {
		t.Fatal("changed staged bundle modified the installed reporter", err)
	}
	if err = os.WriteFile(filepath.Join(m.PresetPlugin, "ClutchbaseAccess.dll"), []byte("new staged reporter"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.UpdateReporter(ctx, in.Name, source); err != nil {
		t.Fatal(err)
	}
	_, after := m.reporterHashes(in.Name)
	if after != source {
		t.Fatal("installed reporter does not match the requested fingerprint")
	}
	if err = os.WriteFile(filepath.Join(m.PresetPlugin, "ClutchbaseAccess.dll"), []byte("another staged reporter"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = m.UpdateReporter(ctx, in.Name, source); err != nil {
		t.Fatal("an already installed request was not idempotent", err)
	}
	_, after = m.reporterHashes(in.Name)
	if after != source {
		t.Fatal("idempotent replay installed a different build")
	}
}

func TestReporterStatusAndMissingReporterRecovery(t *testing.T) {
	ctx := context.Background()
	m := fixture(t)
	in := create(t, m, "friends", 27015)
	if err := m.Install(ctx, in.Name, false); err != nil {
		t.Fatal(err)
	}
	addPluginFixture(t, m, in.Name)
	presetFixture(t, m)
	if err := m.SetPreset(ctx, in.Name, "match"); err != nil {
		t.Fatal(err)
	}
	info, err := m.ReporterStatus(in.Name)
	if err != nil || info.Status != "current" || info.StagedHash == "" || info.StagedHash != info.InstalledHash {
		t.Fatalf("current reporter: %+v, %v", info, err)
	}
	if err := os.Remove(filepath.Join(m.presetPluginDir(in.Name), "ClutchbaseAccess.dll")); err != nil {
		t.Fatal(err)
	}
	info, err = m.ReporterStatus(in.Name)
	if err != nil || info.Status != "not_installed" || info.InstalledHash != "" {
		t.Fatalf("missing reporter: %+v, %v", info, err)
	}
	if err := m.UpdateReporter(ctx, in.Name, info.StagedHash); err != nil {
		t.Fatal("cannot restore missing reporter", err)
	}
	info, err = m.ReporterStatus(in.Name)
	if err != nil || info.Status != "current" {
		t.Fatalf("restored reporter: %+v, %v", info, err)
	}
	if err := os.Remove(filepath.Join(m.PresetPlugin, "ClutchbaseAccess.dll")); err != nil {
		t.Fatal(err)
	}
	info, err = m.ReporterStatus(in.Name)
	if err != nil || info.Status != "unavailable" || info.InstalledHash == "" {
		t.Fatalf("unavailable staged reporter: %+v, %v", info, err)
	}
	if _, err := m.ReporterStatus("../outside"); err == nil {
		t.Fatal("accepted an invalid instance name")
	}
}
