package control

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceRequestsExcludeConflictingOperations(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_game_update,supports_reporter_update,supports_removal,supports_presets,supports_whitelist) VALUES('machine','Machine','localhost','token',now(),true,true,true,true,true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen,reporter_source_hash) VALUES('server','Server','localhost',27015,'','machine','stopped','stopped','managed',now(),repeat('a',64))`); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w.Code
	}
	if code := call("POST", "/api/hosts/server/game-update", `{"revision":1}`); code != 202 {
		t.Fatalf("game update: %d", code)
	}
	for _, request := range []struct{ method, path, body string }{
		{"POST", "/api/hosts/server/reporter-update", `{"revision":2}`},
		{"PATCH", "/api/hosts/server", `{"revision":2,"desiredState":"running"}`},
		{"PATCH", "/api/hosts/server", `{"revision":2,"map":"de_nuke"}`},
		{"PATCH", "/api/hosts/server", `{"revision":2,"whitelist":[]}`},
		{"PATCH", "/api/hosts/server", `{"revision":2,"preset":"match"}`},
		{"DELETE", "/api/hosts/server", `{"revision":2}`},
	} {
		if code := call(request.method, request.path, request.body); code != 409 {
			t.Fatalf("%s %s during game update: %d", request.method, request.path, code)
		}
	}
	if _, err := a.DB.Exec(ctx, `UPDATE game_updates SET status='complete' WHERE host_id='server'`); err != nil {
		t.Fatal(err)
	}
	if code := call("POST", "/api/hosts/server/reporter-update", `{"revision":2}`); code != 202 {
		t.Fatalf("reporter update: %d", code)
	}
	if code := call("POST", "/api/hosts/server/game-update", `{"revision":3}`); code != 409 {
		t.Fatalf("game update during reporter: %d", code)
	}
}

func TestMaintenanceStateIsRereadAfterWaitingForHostLock(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,last_seen,supports_game_update,supports_reporter_update) VALUES('machine','Machine','localhost','token',now(),true,true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,desired_state,observed_state,driver,last_seen,reporter_source_hash) VALUES('server','Server','localhost',27015,'','machine','stopped','stopped','managed',now(),repeat('a',64))`); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM hosts WHERE id='server' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("POST", "/api/hosts/server/reporter-update", strings.NewReader(`{"revision":2}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err = a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM hosts WHERE id=%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("reporter request did not wait for host lock")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO game_updates(host_id,request_id,status) VALUES('server','ABCDEFGHIJKLMNOPQRSTUVWX23','pending'); UPDATE hosts SET revision=2 WHERE id='server'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != 409 {
			t.Fatal(fmt.Sprintf("stale maintenance snapshot accepted: %d", code))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
}
