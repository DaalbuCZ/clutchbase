package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
)

func TestAccessApprovalEmptyRosterEligibility(t *testing.T) {
	now := time.Now()
	base := protocol.Host{MachineID: "machine", CanWhitelist: true, LastSeen: &now, Online: true, DesiredState: "stopped", ObservedState: "stopped", Actions: protocol.Actions{ChangeWhitelist: true}}
	for _, roster := range [][]string{nil, {}} {
		host := base
		host.Whitelist = roster
		if ok, blocker := accessApprovalState(host, "76561198154367261"); !ok || blocker != "" {
			t.Fatalf("empty roster should be otherwise eligible: ok=%v blocker=%q", ok, blocker)
		}
		if got := len(host.Whitelist) == 0; !got {
			t.Fatal("empty roster must require explicit whitelist enablement")
		}
	}
	for name, set := range map[string]func(*protocol.Host){
		"game update":     func(h *protocol.Host) { h.GameUpdateStatus = "pending" },
		"reporter update": func(h *protocol.Host) { h.ReporterUpdateStatus = "pending" },
		"mode command":    func(h *protocol.Host) { h.ModeCommandStatus = "pending" },
		"match control":   func(h *protocol.Host) { h.MatchControlStatus = "pending" },
		"restart":         func(h *protocol.Host) { h.RestartPhase = "stopping" },
	} {
		host := base
		host.Whitelist = []string{"76561198154367262"}
		set(&host)
		if ok, blocker := accessApprovalState(host, "76561198154367261"); ok || blocker != "Server maintenance is in progress." {
			t.Fatalf("%s should block approval: ok=%v blocker=%q", name, ok, blocker)
		}
	}
}

func TestAccessRequestsOnboardAndApprove(t *testing.T) {
	a := integrationAPI(t)
	ctx := t.Context()
	steam := "76561198154367261"
	sessionToken := strings.Repeat("s", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,'Player') ON CONFLICT DO NOTHING`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(sessionToken), steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_whitelist) VALUES('machine','Machine','127.0.0.1','hash','managed',now(),true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,last_seen,desired_state,observed_state,whitelist,whitelist_hash) VALUES('share-host','Friends','127.0.0.1',27015,'','machine','managed',now(),'stopped','stopped',ARRAY['76561198154367262']::text[],$1)`, protocol.WhitelistHash([]string{"76561198154367262"})); err != nil {
		t.Fatal(err)
	}
	h := a.Handler()
	call := func(method, path string, body any, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(data))
		req.Header.Set("Origin", a.Origin)
		if admin {
			req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		} else {
			req.AddCookie(&http.Cookie{Name: playerCookie, Value: sessionToken})
		}
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	if got := call(http.MethodPost, "/api/player/access-requests", map[string]string{"hostId": "share-host"}, false); got.Code != http.StatusOK {
		t.Fatalf("request: %d %s", got.Code, got.Body.String())
	}
	var own []playerAccessRequest
	if got := call(http.MethodGet, "/api/player/access-requests", nil, false); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &own) != nil || len(own) != 1 || own[0].HostName != "Friends" || own[0].Status != "pending" {
		t.Fatalf("player request projection: %d %s %+v", got.Code, got.Body.String(), own)
	}
	var admin []adminAccessRequest
	if got := call(http.MethodGet, "/api/access-requests", nil, true); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &admin) != nil || len(admin) != 1 || !admin[0].CanApprove || admin[0].RequiresWhitelistEnable || admin[0].SteamID != steam || admin[0].GameName != "Player" {
		t.Fatalf("admin request projection: %d %s %+v", got.Code, got.Body.String(), admin)
	}
	if got := call(http.MethodPatch, "/api/access-requests/"+own[0].ID, map[string]any{"revision": 1, "action": "approve"}, true); got.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", got.Code, got.Body.String())
	}
	var roster []string
	var hostRevision int64
	if err := a.DB.QueryRow(ctx, "SELECT whitelist,revision FROM hosts WHERE id='share-host'").Scan(&roster, &hostRevision); err != nil || len(roster) != 2 || !containsSteamID(roster, steam) || !containsSteamID(roster, "76561198154367262") || hostRevision != 2 {
		t.Fatalf("approval did not update intent: %v rev=%d roster=%v", err, hostRevision, roster)
	}
	var status string
	if err := a.DB.QueryRow(ctx, "SELECT status FROM access_requests WHERE id=$1", own[0].ID).Scan(&status); err != nil || status != "approved" {
		t.Fatalf("approval status: %s %v", status, err)
	}
	// Duplicate submits are idempotent after approval and do not create a
	// second private record.
	if got := call(http.MethodPost, "/api/player/access-requests", map[string]string{"hostId": "share-host"}, false); got.Code != http.StatusOK {
		t.Fatalf("duplicate request: %d %s", got.Code, got.Body.String())
	}
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET whitelist=ARRAY['76561198154367262']::text[] WHERE id='share-host'"); err != nil {
		t.Fatal(err)
	}
	if got := call(http.MethodGet, "/api/player/access-requests", nil, false); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &own) != nil || len(own) != 1 || own[0].Status != "revoked" {
		t.Fatalf("revoked projection: %d %s %+v", got.Code, got.Body.String(), own)
	}
	if got := call(http.MethodPost, "/api/player/access-requests", map[string]string{"hostId": "share-host"}, false); got.Code != http.StatusOK {
		t.Fatalf("resubmit revoked request: %d %s", got.Code, got.Body.String())
	}
	if got := call(http.MethodGet, "/api/player/access-requests", nil, false); got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &own) != nil || own[0].Status != "pending" || own[0].Revision != 3 {
		t.Fatalf("resubmitted request: %d %s %+v", got.Code, got.Body.String(), own)
	}
}

func TestAccessApprovalExplicitlyEnablesEmptyWhitelist(t *testing.T) {
	a := integrationAPI(t)
	ctx := t.Context()
	steam := "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,'Player')`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,driver,last_seen,supports_whitelist) VALUES('machine','Machine','127.0.0.1','hash','managed',now(),true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,driver,last_seen,desired_state,observed_state,whitelist) VALUES('host','Friends','127.0.0.1',27015,'','machine','managed',now(),'stopped','stopped',ARRAY[]::text[])`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO access_requests(id,host_id,steam_id) VALUES('request','host',$1)`, steam); err != nil {
		t.Fatal(err)
	}
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/access-requests/request", strings.NewReader(body))
		req.Header.Set("Origin", a.Origin)
		req.Header.Set("Authorization", "Bearer "+a.AdminToken)
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		return w
	}
	var snapshot []adminAccessRequest
	get := httptest.NewRequest(http.MethodGet, "/api/access-requests", nil)
	get.Header.Set("Authorization", "Bearer "+a.AdminToken)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, get)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &snapshot) != nil || len(snapshot) != 1 || !snapshot[0].RequiresWhitelistEnable || !snapshot[0].CanApprove {
		t.Fatalf("empty roster snapshot: %d %s %+v", w.Code, w.Body.String(), snapshot)
	}
	if got := call(`{"revision":1,"action":"approve"}`); got.Code != http.StatusConflict {
		t.Fatalf("missing enablement flag: %d %s", got.Code, got.Body.String())
	}
	if got := call(`{"revision":1,"action":"approve","enableWhitelist":"true"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("malformed enablement flag: %d %s", got.Code, got.Body.String())
	}
	if got := call(`{"revision":1,"action":"approve","enableWhitelist":true}`); got.Code != http.StatusOK {
		t.Fatalf("explicit enablement: %d %s", got.Code, got.Body.String())
	}
	var roster []string
	var desired, status string
	if err := a.DB.QueryRow(ctx, `SELECT whitelist,desired_state FROM hosts WHERE id='host'`).Scan(&roster, &desired); err != nil || len(roster) != 1 || roster[0] != steam || desired != "stopped" {
		t.Fatalf("empty roster enablement changed unsafe state: %v roster=%v desired=%s", err, roster, desired)
	}
	if err := a.DB.QueryRow(ctx, `SELECT status FROM access_requests WHERE id='request'`).Scan(&status); err != nil || status != "approved" {
		t.Fatalf("request status: %s %v", status, err)
	}
}
