package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGameUpdateCommandUsesAuthoritativeMachineState(t *testing.T) {
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
	a := &API{DB: db, AdminToken: strings.Repeat("a", 32)}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_game_update) VALUES('machine','machine','127.0.0.1',$1,now(),true)`, digest("machine-secret")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,map,desired_state,observed_state,driver,last_seen) VALUES('server','server','127.0.0.1',27245,'','machine','de_mirage','stopped','stopped','managed',now())`); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/hosts/server/game-update", bytes.NewBufferString(`{"revision":1}`))
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 202 {
		t.Fatalf("update request status=%d body=%s", w.Code, w.Body.String())
	}

	report := protocol.MachineReport{Driver: "managed", SupportsGameUpdate: true, Servers: []protocol.ServerReport{{ID: "server", Report: protocol.Report{Driver: "managed", State: "stopped", JoinPassword: "private-password"}}}}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer machine-secret")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("heartbeat status=%d body=%s", w.Code, w.Body.String())
	}
	var desired protocol.MachineDesired
	if err := json.Unmarshal(w.Body.Bytes(), &desired); err != nil {
		t.Fatal(err)
	}
	if len(desired.Servers) != 1 || desired.Servers[0].GameUpdate == nil || !protocol.ValidCommandID(desired.Servers[0].GameUpdate.ID) {
		t.Fatalf("missing authoritative game update request: %+v", desired.Servers)
	}
	req = httptest.NewRequest("GET", "/api/hosts/server/join-password", nil)
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	var password map[string]string
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &password) != nil || password["password"] != "private-password" {
		t.Fatalf("admin password retrieval status=%d body=%s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest("GET", "/api/state", nil)
	req.Header.Set("Authorization", "Bearer "+a.AdminToken)
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("private-password")) {
		t.Fatalf("workspace leaked join password, status=%d", w.Code)
	}
	report.Servers[0].GameUpdateID = desired.Servers[0].GameUpdate.ID
	report.Servers[0].GameUpdateStatus = "complete"
	body, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer machine-secret")
	w = httptest.NewRecorder()
	a.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("update result status=%d body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.QueryRow(ctx, "SELECT status FROM game_updates WHERE host_id='server'").Scan(&status); err != nil || status != "complete" {
		t.Fatalf("update status=%q err=%v", status, err)
	}
}
