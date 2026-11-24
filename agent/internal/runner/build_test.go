package runner

import (
	"clutchbase/agent/internal/manager"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCheckCachesSteamLookup(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "steamcmd")
	count := filepath.Join(root, "calls")
	content := "#!/bin/sh\nprintf x >> '" + count + "'\nprintf '%s\\n' '\"730\" { \"depots\" { \"branches\" { \"public\" { \"buildid\" \"456\" } } } }'\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(root, script)
	if err != nil {
		t.Fatal(err)
	}
	f := &Fleet{manager: m}
	f.scheduleBuildCheck()
	f.buildWG.Wait()
	if f.latestBuild != "456" || f.buildCheckedAt.IsZero() {
		t.Fatal("public build was not cached")
	}
	f.scheduleBuildCheck()
	f.buildWG.Wait()
	data, err := os.ReadFile(count)
	if err != nil || string(data) != "x" {
		t.Fatalf("SteamCMD called more than once: %q, %v", data, err)
	}
}
