package manager

import (
	"clutchbase/internal/protocol"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkshopMaps(t *testing.T) {
	root := t.TempDir()
	m := &Manager{Root: root}
	base := filepath.Join(root, "instances", "test", "server", "game", "csgo", "maps", "workshop")
	if err := os.MkdirAll(filepath.Join(base, "123"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"de_ok.bsp", "bad name.bsp", "de_other.vpk"} {
		if err := os.WriteFile(filepath.Join(base, "123", name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "escape.bsp"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "456")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "escape.bsp"), filepath.Join(base, "123", "link.bsp")); err != nil {
		t.Fatal(err)
	}
	got, err := m.WorkshopMaps("test")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"workshop/123/de_ok"}) {
		t.Fatalf("maps: %v", got)
	}
	if _, err := m.WorkshopMaps("../escape"); err == nil {
		t.Fatal("accepted invalid instance")
	}
}

func TestWorkshopLocalFileCheck(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	if err := m.CheckInstalledMap("test", "de_mirage"); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckInstalledMap("test", "workshop/1/custom"); err == nil {
		t.Fatal("missing file accepted")
	}
	base := filepath.Join(m.Root, "instances", "test", "server", "game", "csgo", "maps", "workshop", "1")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(base, "custom.bsp")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckInstalledMap("test", "workshop/1/custom"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckInstalledMap("test", "workshop/1/custom"); err == nil {
		t.Fatal("removed file accepted")
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(base, "..", "2")); err != nil {
		t.Fatal(err)
	}
	if err := m.CheckInstalledMap("test", "workshop/2/custom"); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := m.writeConfig(Instance{Name: "test"}, "workshop/1/custom"); err == nil {
		t.Fatal("config written without map")
	}
	setup := protocol.MatchSetup{ID: "ABCDEFGHIJKLMNOPQRSTUVWX23", Map: "workshop/1/custom", PlayersPerSide: 1, CT: []protocol.MatchSetupPlayer{{SteamID: "76561198154367261", GameName: "CT"}}, T: []protocol.MatchSetupPlayer{{SteamID: "76561198154367262", GameName: "T"}}}
	if err := m.RequestMatchSetup(context.Background(), "test", setup); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("setup bypassed file check: %v", err)
	}

}
