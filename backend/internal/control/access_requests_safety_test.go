package control

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccessRequestPrivacyDecisionsAndExpiry(t *testing.T) {
	a := integrationAPI(t)
	ctx := t.Context()
	const player = "76561198154367261"
	const other = "76561198154367262"
	token := strings.Repeat("s", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'),('76561198154367262'); INSERT INTO hosts(id,name,address,port,token_hash,last_seen,whitelist) VALUES('old','Old','localhost',27015,'',now()-interval '5 minutes',ARRAY['76561198154367262']),('fresh','Fresh','localhost',27025,'',now(),ARRAY['76561198154367262']); INSERT INTO access_requests(id,host_id,steam_id) VALUES('own','old','76561198154367261'),('private','fresh','76561198154367262')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), player); err != nil {
		t.Fatal(err)
	}
	call := func(method, path, body, origin string, admin, session bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		if admin {
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		}
		if session {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "/api/player/access-requests", "", a.Origin, false, true); w.Code != 200 || strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), other) {
		t.Fatalf("privacy %d %s", w.Code, w.Body.String())
	}
	if w := call("POST", "/api/player/access-requests", `{"hostId":"fresh"}`, "https://attacker.invalid", false, true); w.Code != 403 {
		t.Fatalf("origin %d", w.Code)
	}
	if w := call("POST", "/api/player/access-requests", `{"hostId":"fresh"}`, a.Origin, false, false); w.Code != 401 {
		t.Fatalf("anonymous %d", w.Code)
	}
	if w := call("PATCH", "/api/access-requests/own", `{"revision":1,"action":"reject"}`, a.Origin, false, true); w.Code != 401 {
		t.Fatalf("nonadmin %d", w.Code)
	}
	if w := call("PATCH", "/api/access-requests/own", `{"revision":2,"action":"reject"}`, a.Origin, true, false); w.Code != 409 {
		t.Fatalf("stale %d", w.Code)
	}
	if w := call("PATCH", "/api/access-requests/own", `{"revision":1,"action":"reject"}`, a.Origin, true, false); w.Code != 200 {
		t.Fatalf("reject %d %s", w.Code, w.Body.String())
	}
	if w := call("POST", "/api/player/access-requests", `{"hostId":"old"}`, a.Origin, false, true); w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"pending"`) {
		t.Fatalf("resubmit %d %s", w.Code, w.Body.String())
	}
	at := a.accessRequestExpiry(ctx, time.Now())
	if at.IsZero() || time.Until(at) < 10*time.Second || time.Until(at) > 21*time.Second {
		t.Fatalf("expiry must ignore stale heartbeat: %v", at)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE hosts SET last_seen=now()-interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	if at = a.accessRequestExpiry(ctx, time.Now()); !at.IsZero() {
		t.Fatalf("expired heartbeats must not schedule a busy loop: %v", at)
	}
}

func TestAccessApprovalPreservesConcurrentRosterEdit(t *testing.T) {
	a := integrationAPI(t)
	ctx := t.Context()
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'); INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_whitelist) VALUES('machine','Machine','localhost','hash','managed',now(),true); INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,last_seen,desired_state,observed_state,whitelist,whitelist_hash) VALUES('host','Host','localhost',27015,'','machine','managed',now(),'stopped','stopped',ARRAY['76561198154367262']::text[],''); INSERT INTO access_requests(id,host_id,steam_id) VALUES('request','host','76561198154367261')`); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM hosts WHERE id='host' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("PATCH", "/api/access-requests/request", strings.NewReader(`{"revision":1,"action":"approve"}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(5 * time.Second)
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
		t.Fatal("approval did not wait for the host lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET whitelist=ARRAY['76561198154367262','76561198154367263']::text[],revision=revision+1 WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got != 200 {
			t.Fatalf("approval %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not finish")
	}
	var roster []string
	if err = a.DB.QueryRow(ctx, `SELECT whitelist FROM hosts WHERE id='host'`).Scan(&roster); err != nil || len(roster) != 3 || !containsSteamID(roster, "76561198154367261") || !containsSteamID(roster, "76561198154367262") || !containsSteamID(roster, "76561198154367263") {
		t.Fatalf("roster overwritten: %v %v", roster, err)
	}
}

func TestAccessApprovalRejectsStaleEmptyRosterWithoutEnablement(t *testing.T) {
	a := integrationAPI(t)
	ctx := t.Context()
	steam := "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES($1)`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_whitelist) VALUES('machine','Machine','localhost','hash','managed',now(),true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,last_seen,desired_state,observed_state,whitelist) VALUES('host','Host','localhost',27015,'','machine','managed',now(),'stopped','stopped',ARRAY['76561198154367262']::text[])`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO access_requests(id,host_id,steam_id) VALUES('request','host',$1)`, steam); err != nil {
		t.Fatal(err)
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM hosts WHERE id='host' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req := httptest.NewRequest("PATCH", "/api/access-requests/request", strings.NewReader(`{"revision":1,"action":"approve"}`))
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		result <- w.Code
	}()
	deadline := time.Now().Add(5 * time.Second)
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
		t.Fatal("approval did not wait for the host lock")
	}
	if _, err = tx.Exec(ctx, `UPDATE hosts SET whitelist=NULL,revision=revision+1 WHERE id='host'`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-result:
		if got != http.StatusConflict {
			t.Fatalf("stale empty roster was approved: %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not finish")
	}
	var status string
	if err = a.DB.QueryRow(ctx, `SELECT status FROM access_requests WHERE id='request'`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("stale approval changed request: %s %v", status, err)
	}
}
