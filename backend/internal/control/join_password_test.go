package control

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
)

func TestJoinPasswordCacheExpiresAndClears(t *testing.T) {
	a := &API{}
	a.rememberJoinPassword("server", "private-password")
	if got, ok := a.cachedJoinPassword("server"); !ok || got != "private-password" {
		t.Fatalf("cached password = %q, %v", got, ok)
	}
	a.joinPasswordsMu.Lock()
	entry := a.joinPasswords["server"]
	entry.received = time.Now().Add(-joinPasswordFreshness)
	a.joinPasswords["server"] = entry
	a.joinPasswordsMu.Unlock()
	if _, ok := a.cachedJoinPassword("server"); ok {
		t.Fatal("stale password was returned")
	}
	a.rememberJoinPassword("server", "private-password")
	a.rememberJoinPassword("server", "")
	if _, ok := a.cachedJoinPassword("server"); ok {
		t.Fatal("cleared password was returned")
	}
}

func TestJoinPasswordNeverEntersWorkspaceSnapshot(t *testing.T) {
	data, err := json.Marshal(protocol.Host{ID: "server"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "joinPassword") || strings.Contains(string(data), "private-password") {
		t.Fatalf("workspace leaked a join password: %s", data)
	}
}
