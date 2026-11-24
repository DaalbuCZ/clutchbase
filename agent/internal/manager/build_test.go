package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildParser(t *testing.T) {
	manifest := []byte(`"AppState" { "appid" "730" "buildid" "12345" }`)
	if got := vdfValue(manifest, "AppState", "buildid"); got != "12345" {
		t.Fatalf("manifest build = %q", got)
	}
	info := []byte(`"730" { "depots" { "branches" { "beta" { "buildid" "999" } "public" { "buildid" "67890" } } } }`)
	if got := vdfValue(info, "branches", "public", "buildid"); got != "67890" {
		t.Fatalf("public build = %q", got)
	}
	if got := vdfValue([]byte(`"AppState" { "buildid" "../../bad" }`), "AppState", "buildid"); got != "" {
		t.Fatalf("invalid build = %q", got)
	}
}
func TestInstalledAndPublicBuild(t *testing.T) {
	root := t.TempDir()
	m, err := New(root, filepath.Join(root, "steamcmd"))
	if err != nil {
		t.Fatal(err)
	}
	name := "s-aaaaaaaaaaaaaaaaaaaaaaaaaa"
	dir, err := m.Dir(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "server", "steamapps"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "server", "steamapps", "appmanifest_730.acf"), []byte(`"AppState" { "buildid" "12345" }`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := m.InstalledBuild(name); got != "12345" {
		t.Fatalf("installed=%q", got)
	}
	script := "#!/bin/sh\nprintf '%s\\n' '\"730\" { \"depots\" { \"branches\" { \"public\" { \"buildid\" \"67890\" } } } }'\n"
	if err := os.WriteFile(m.SteamCMD, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := m.PublicBuild(context.Background())
	if err != nil || got != "67890" {
		t.Fatalf("public=%q, %v", got, err)
	}
}
