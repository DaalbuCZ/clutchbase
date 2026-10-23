package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBoundaries(t *testing.T) {
	a := &API{AdminToken: strings.Repeat("a", 32)}
	r := httptest.NewRequest("GET", "/api/state", nil)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("unauthenticated request: %d", w.Code)
	}
	r = httptest.NewRequest("GET", "/api/state", nil)
	r.Header.Set("Authorization", a.AdminToken)
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("accepted a credential without the Bearer scheme")
	}
	for _, address := range []string{"host;quit", "example.com/path", "127.0.0.1:27015", "-option", ""} {
		if validAddress(address) {
			t.Fatalf("accepted %q", address)
		}
	}
	for _, address := range []string{"play.example.com", "127.0.0.1", "::1"} {
		if !validAddress(address) {
			t.Fatalf("rejected %q", address)
		}
	}
}

func TestPostgresControlLoop(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run real PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "test_" + strings.ToLower(rand.Text())
	schemaID := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaID); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schemaID+" CASCADE")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schemaName
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	token := strings.Repeat("a", 32)
	api := &API{DB: db, AdminToken: token}
	if err := api.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := api.Migrate(ctx); err != nil {
		t.Fatal("schema setup is not idempotent:", err)
	}
	handler := api.Handler()
	call := func(method, path, auth string, body any, want int) []byte {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer "+auth)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	var created struct {
		Host       protocol.Host `json:"host"`
		AgentToken string        `json:"agentToken"`
	}
	if err := json.Unmarshal(call("POST", "/api/hosts", token, map[string]any{"name": "Friends", "address": "127.0.0.1", "port": 27015}, 201), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Host.ID
	path := "/api/hosts/" + id
	heartbeat := "/api/agent/" + id + "/heartbeat"
	var storedHash string
	if err := db.QueryRow(ctx, "SELECT token_hash FROM hosts WHERE id=$1", id).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == created.AgentToken || storedHash != digest(created.AgentToken) {
		t.Fatal("agent token was not hashed")
	}
	state := func() protocol.Host {
		t.Helper()
		var workspace protocol.Workspace
		if err := json.Unmarshal(call("GET", "/api/state", token, nil, 200), &workspace); err != nil {
			t.Fatal(err)
		}
		if len(workspace.Hosts) != 1 {
			t.Fatal("expected one host")
		}
		return workspace.Hosts[0]
	}
	patch := func(rev int64, desired string, want int) {
		t.Helper()
		call("PATCH", path, token, map[string]any{"revision": rev, "desiredState": desired}, want)
	}
	report := func(observed string) {
		t.Helper()
		call("POST", heartbeat, created.AgentToken, protocol.Report{State: observed, Driver: "simulated", ActiveMap: "de_mirage"}, 200)
	}
	if state().Status != "offline" {
		t.Fatal("unlinked host not offline")
	}
	patch(1, "running", 409)
	call("POST", heartbeat, token, protocol.Report{State: "stopped", Driver: "simulated"}, 401)
	call("GET", "/api/state", created.AgentToken, nil, 401)
	report("stopped")
	patch(1, "running", 200)
	if got := state(); got.Status != "starting" || got.ObservedState != "stopped" {
		t.Fatalf("UI would report a premature start: %+v", got)
	}
	patch(1, "stopped", 409)
	report("running")
	if state().Status != "running" {
		t.Fatal("did not observe running")
	}
	call("PATCH", path, token, map[string]any{"revision": 2, "map": "de_nuke"}, 409)
	patch(2, "stopped", 200)
	if state().Status != "stopping" {
		t.Fatal("UI would report a premature stop")
	}
	report("stopped")
	call("PATCH", path, token, map[string]any{"revision": 3, "map": "de_nuke"}, 200)
	call("PATCH", path, token, map[string]any{"revision": 4, "map": "de_nuke; quit"}, 400)
	call("PATCH", path, token, map[string]any{"revision": 4, "desiredState": "running", "unit": "evil"}, 400)
	handler = (&API{DB: db, AdminToken: token}).Handler()
	if got := state(); got.Map != "de_nuke" || got.Revision != 4 {
		t.Fatalf("state not persisted: %+v", got)
	}
	if _, err := db.Exec(ctx, "UPDATE hosts SET last_seen=now()-interval '30 seconds' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if state().Status != "offline" {
		t.Fatal("stale agent not offline")
	}
	patch(4, "running", 409)
	call(http.MethodGet, "/api/missing", token, nil, 404)
}
