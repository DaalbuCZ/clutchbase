package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	// Adopt a database made by the original unversioned release, preserving rows.
	if _, err := db.Exec(ctx, schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO hosts (id,name,address,port,token_hash,map,revision) VALUES ('upgrade','Existing','localhost',27015,'old-hash','de_dust2',7)"); err != nil {
		t.Fatal(err)
	}
	if err := api.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := api.Migrate(ctx); err != nil {
		t.Fatal("schema setup is not idempotent:", err)
	}
	var oldMap, oldHash string
	var oldRevision int
	if err := db.QueryRow(ctx, "SELECT map,token_hash,revision FROM hosts WHERE id='upgrade'").Scan(&oldMap, &oldHash, &oldRevision); err != nil || oldMap != "de_dust2" || oldHash != "old-hash" || oldRevision != 7 {
		t.Fatalf("migration damaged existing server: %s %s %d %v", oldMap, oldHash, oldRevision, err)
	}
	var versions int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&versions); err != nil || versions != 2 {
		t.Fatal("migrations not tracked", err)
	}
	if _, err := db.Exec(ctx, "DELETE FROM hosts WHERE id='upgrade'"); err != nil {
		t.Fatal(err)
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
	var desired protocol.Desired
	if err := json.Unmarshal(call("POST", heartbeat, created.AgentToken, protocol.Report{State: "uninstalled", Driver: "managed"}, 200), &desired); err != nil {
		t.Fatal(err)
	}
	if desired.Port != 27015 || desired.Map != "de_nuke" {
		t.Fatal("provisioning instructions do not match persisted instance")
	}
	if got := state(); got.Status != "uninstalled" || !got.Actions.Start || !got.Actions.ChangeMap {
		t.Fatalf("uninstalled instance is not actionable: %+v", got)
	}
	patch(4, "running", 200)
	call("POST", heartbeat, created.AgentToken, protocol.Report{State: "installing", Driver: "managed"}, 200)
	if got := state(); got.Status != "installing" || !got.Actions.Stop || got.Actions.ChangeMap {
		t.Fatalf("invalid installation state: %+v", got)
	}
	call("PATCH", path, token, map[string]any{"revision": 5, "map": "de_mirage"}, 409)
	patch(5, "stopped", 200)
	if state().Status != "stopping" {
		t.Fatal("cancelled install prematurely reported stopped")
	}
	call("POST", heartbeat, created.AgentToken, protocol.Report{State: "uninstalled", Driver: "managed"}, 200)
	if state().Status != "uninstalled" {
		t.Fatal("cancelled install did not return to uninstalled")
	}
	if _, err := db.Exec(ctx, "UPDATE hosts SET last_seen=now()-interval '30 seconds' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	if state().Status != "offline" {
		t.Fatal("stale agent not offline")
	}
	patch(6, "running", 409)
	call(http.MethodGet, "/api/missing", token, nil, 404)
	// Connect one machine, then create multiple servers without new agent credentials.
	type registration struct {
		Machine    protocol.Machine `json:"machine"`
		AgentToken string           `json:"agentToken"`
	}
	var machine, other registration
	json.Unmarshal(call("POST", "/api/machines", token, map[string]any{"name": "Home", "address": "10.9.0.1"}, 201), &machine)
	json.Unmarshal(call("POST", "/api/machines", token, map[string]any{"name": "Other", "address": "10.9.0.2"}, 201), &other)
	call("POST", "/api/machines", token, map[string]any{"name": "Duplicate", "address": "10.9.0.1"}, 409)
	if err := db.QueryRow(ctx, "SELECT token_hash FROM machines WHERE id=$1", machine.Machine.ID).Scan(&storedHash); err != nil || storedHash != digest(machine.AgentToken) {
		t.Fatal("machine credential not hashed", err)
	}
	fleetPath := "/api/machine-agent/" + machine.Machine.ID + "/heartbeat"
	createPath := "/api/machines/" + machine.Machine.ID + "/servers"
	createBody := map[string]any{"name": "Friday", "map": "de_nuke", "port": 0}
	call("POST", createPath, token, createBody, 409)
	call("POST", fleetPath, token, protocol.MachineReport{Driver: "simulated"}, 401)
	call("POST", fleetPath, other.AgentToken, protocol.MachineReport{Driver: "simulated"}, 401)
	call("GET", "/api/state", machine.AgentToken, nil, 401)
	call("POST", createPath, machine.AgentToken, createBody, 401)
	call("POST", fleetPath, machine.AgentToken, protocol.MachineReport{Driver: "simulated"}, 200)
	var first, second protocol.Host
	json.Unmarshal(call("POST", createPath, token, createBody, 201), &first)
	json.Unmarshal(call("POST", createPath, token, createBody, 201), &second)
	if first.Port != 27015 || second.Port != 27025 || first.MachineID != machine.Machine.ID || first.Online {
		t.Fatalf("invalid instance registration: %+v %+v", first, second)
	}
	call("POST", createPath, token, map[string]any{"name": "Overlap", "map": "de_nuke", "port": 27017}, 409)
	call("POST", createPath, token, map[string]any{"name": "Bad", "map": "de_nuke", "port": 65534}, 400)
	call("POST", createPath, token, map[string]any{"name": "Bad", "map": "de_nuke", "root": "/tmp/arbitrary"}, 400)
	call("POST", "/api/agent/"+first.ID+"/heartbeat", machine.AgentToken, protocol.Report{Driver: "simulated", State: "stopped"}, 401)
	var fleetDesired protocol.MachineDesired
	reportFleet := protocol.MachineReport{Driver: "simulated", Servers: []protocol.ServerReport{
		{ID: first.ID, Report: protocol.Report{Driver: "simulated", State: "stopped"}},
		{ID: second.ID, Report: protocol.Report{Driver: "simulated", State: "uninstalled"}},
	}}
	json.Unmarshal(call("POST", fleetPath, machine.AgentToken, reportFleet, 200), &fleetDesired)
	if len(fleetDesired.Servers) != 2 || fleetDesired.Servers[0].ID != first.ID || fleetDesired.Servers[0].Port != 27015 || fleetDesired.Servers[0].Map != "de_nuke" {
		t.Fatalf("wrong fleet instructions: %+v", fleetDesired)
	}
	call("PATCH", "/api/hosts/"+first.ID, token, map[string]any{"revision": 1, "desiredState": "running"}, 200)
	call("PATCH", "/api/hosts/"+first.ID, token, map[string]any{"revision": 1, "desiredState": "stopped"}, 409)
	// Foreign IDs reject the entire heartbeat, including otherwise valid sibling reports.
	reportFleet.Servers[0].State = "running"
	reportFleet.Servers[1].ID = id
	call("POST", fleetPath, machine.AgentToken, reportFleet, 400)
	var observed string
	if err := db.QueryRow(ctx, "SELECT observed_state FROM hosts WHERE id=$1", first.ID).Scan(&observed); err != nil || observed != "stopped" {
		t.Fatal("foreign report partially committed", err)
	}
	reportFleet.Servers[1].ID = second.ID
	call("POST", fleetPath, machine.AgentToken, reportFleet, 200)
	reportFleet.Servers[1].ID = first.ID
	call("POST", fleetPath, machine.AgentToken, reportFleet, 400)
	// Concurrent requests serialize allocation on the machine row.
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data, _ := json.Marshal(map[string]any{"name": fmt.Sprintf("Server %d", i), "map": "de_mirage", "port": 0})
			req := httptest.NewRequest("POST", createPath, bytes.NewReader(data))
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			results <- res
		}(i)
	}
	wg.Wait()
	close(results)
	ports := map[int]bool{first.Port: true, second.Port: true}
	for res := range results {
		if res.Code != 201 {
			t.Fatalf("concurrent creation: %d %s", res.Code, res.Body.String())
		}
		var h protocol.Host
		json.Unmarshal(res.Body.Bytes(), &h)
		if ports[h.Port] {
			t.Fatal("concurrent port collision")
		}
		ports[h.Port] = true
	}
	var workspace protocol.Workspace
	json.Unmarshal(call("GET", "/api/state", token, nil, 200), &workspace)
	if len(workspace.Machines) != 2 || !workspace.Machines[0].CreateServer || !workspace.Machines[0].Online || workspace.Machines[1].CreateServer {
		t.Fatalf("wrong machine actions: %+v", workspace.Machines)
	}
	if _, err := db.Exec(ctx, "UPDATE machines SET last_seen=now()-interval '30 seconds' WHERE id=$1", machine.Machine.ID); err != nil {
		t.Fatal(err)
	}
	call("POST", createPath, token, createBody, 409)

	// An online machine must not make omitted/stale instance reports look fresh.
	if _, err := db.Exec(ctx, "UPDATE hosts SET last_seen=now()-interval '30 seconds' WHERE machine_id=$1", machine.Machine.ID); err != nil {
		t.Fatal(err)
	}
	call("POST", fleetPath, machine.AgentToken, protocol.MachineReport{Driver: "simulated"}, 200)
	json.Unmarshal(call("GET", "/api/state", token, nil, 200), &workspace)
	for _, h := range workspace.Hosts {
		if h.MachineID == machine.Machine.ID && (h.Online || h.Actions.Start || h.Actions.Stop) {
			t.Fatal("empty machine heartbeat refreshed instance state")
		}
	}
	// Capacity is computed on the server and rechecked during creation.
	call("POST", "/api/machine-agent/"+other.Machine.ID+"/heartbeat", other.AgentToken, protocol.MachineReport{Driver: "managed"}, 200)
	if _, err := db.Exec(ctx, `INSERT INTO hosts (id,name,address,port,token_hash,machine_id) SELECT 'limit-'||n,'Capacity test','10.9.0.2',27015+n*10,'',$1 FROM generate_series(1,64) n`, other.Machine.ID); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/machines/"+other.Machine.ID+"/servers", token, createBody, 409)
	json.Unmarshal(call("GET", "/api/state", token, nil, 200), &workspace)
	if workspace.Machines[1].CreateServer {
		t.Fatal("UI offered creation beyond capacity")
	}

}
