package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"clutchbase/internal/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationAPI(t *testing.T) *API {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx, cancel := context.WithCancel(context.Background())
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	schema := pgx.Identifier{"test_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		cancel()
		admin.Close(ctx)
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = strings.Trim(schema, "\"")
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		db.Close()
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})
	a := &API{DB: db, AdminToken: strings.Repeat("a", 32), Origin: "http://127.0.0.1:8080", Context: ctx}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return a
}

func liveDial(t *testing.T, url string, token string, headers http.Header) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(url, "http"), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if err := wsjson.Write(ctx, c, map[string]string{"token": token}); err != nil {
		t.Fatal(err)
	}
	return c
}

func readLive(t *testing.T, c *websocket.Conn, target any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Read(ctx, c, target); err != nil {
		t.Fatal(err)
	}
}

func TestLiveAuthenticationAndOrigin(t *testing.T) {
	a := &API{AdminToken: strings.Repeat("a", 32)}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live", "wrong", nil)
	var denied struct {
		Code int `json:"code"`
	}
	readLive(t, c, &denied)
	if denied.Code != 401 {
		t.Fatal("unauthenticated socket received state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if conn, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/live", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://attacker.example"}}}); err == nil {
		conn.CloseNow()
		t.Fatal("accepted cross-origin socket")
	} else if res == nil || res.StatusCode != 403 {
		t.Fatal("wrong origin rejection", err)
	}
}

func TestLivePushCommitExpiryAndLongConnection(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, "INSERT INTO hosts(id,name,address,port,token_hash,last_seen,observed_state) VALUES('live','Live','127.0.0.1',27015,'',now(),'stopped')"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var initial protocol.Workspace
	readLive(t, c, &initial)
	if len(initial.Hosts) != 1 || !initial.Hosts[0].Actions.Start {
		t.Fatal("missing initial canonical snapshot")
	}
	// The normal five-second HTTP timeout must not terminate the live stream.
	time.Sleep(6 * time.Second)
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE hosts SET map='de_nuke' WHERE id='live'"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	received := make(chan protocol.Workspace, 1)
	failure := make(chan error, 1)
	go func() {
		var state protocol.Workspace
		err := wsjson.Read(context.Background(), c, &state)
		if err != nil {
			failure <- err
		} else {
			received <- state
		}
	}()
	select {
	case <-received:
		t.Fatal("rolled-back change was published")
	case err := <-failure:
		t.Fatal(err)
	case <-time.After(100 * time.Millisecond):
	}
	// A direct database write must push without a browser refresh or HTTP mutation.
	if _, err = a.DB.Exec(ctx, "UPDATE hosts SET map='de_inferno',revision=revision+1,last_seen=now()-interval '19 seconds' WHERE id='live'"); err != nil {
		t.Fatal(err)
	}
	select {
	case state := <-received:
		if state.Hosts[0].Map != "de_inferno" || state.Hosts[0].Revision != 2 || !state.Hosts[0].Online {
			t.Fatal("wrong committed snapshot")
		}
	case err := <-failure:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("committed state was not pushed")
	}
	var expired protocol.Workspace
	readLive(t, c, &expired)
	if expired.Hosts[0].Online || expired.Hosts[0].Status != "offline" || expired.Hosts[0].Actions.Start {
		t.Fatal("offline expiry was not pushed without a database mutation")
	}
}

func TestPlayerLiveProjectionRevocationAndSessionExpiry(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	id := "76561198154367261"
	token := rand.Text() + rand.Text()
	if _, err := a.DB.Exec(ctx, "INSERT INTO players(steam_id) VALUES($1)", id); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, "INSERT INTO player_sessions VALUES($1,$2,now()+interval '1 hour')", digest(token), id); err != nil {
		t.Fatal(err)
	}
	roster := []string{id}
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash,supports_whitelist) VALUES('machine','Machine','localhost','',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,machine_id,whitelist,whitelist_hash,observed_state,desired_state,driver,access_status,last_seen,log_tail,error) VALUES('allowed','Allowed','localhost',27015,'','machine',$1,$2,'running','running','managed','verified',now(),'private log',''),('hidden','Hidden','localhost',27025,'',NULL,NULL,'','stopped','stopped','','',now(),'hidden log','')`, roster, protocol.WhitelistHash(roster)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	headers := http.Header{"Cookie": {playerCookie + "=" + token}}
	c := liveDial(t, server.URL+"/api/live?view=player", "", headers)
	var state playerWorkspace
	readLive(t, c, &state)
	if state.SteamID != id || len(state.Servers) != 1 || state.Servers[0].ID != "allowed" || !state.Servers[0].CanJoin {
		t.Fatalf("wrong player view: %+v", state)
	}
	raw, _ := json.Marshal(state)
	for _, forbidden := range []string{"private log", "error", "hidden", "logTail", "whitelist", "machines", "actions"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("player projection exposed %s", forbidden)
		}
	}
	if _, err := a.DB.Exec(ctx, "UPDATE hosts SET whitelist=ARRAY[]::text[] WHERE id='allowed'"); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &state)
	if len(state.Servers) != 0 {
		t.Fatal("roster revocation not pushed")
	}
	if _, err := a.DB.Exec(ctx, "UPDATE player_sessions SET expires_at=now()+interval '1 second' WHERE token_hash=$1", digest(token)); err != nil {
		t.Fatal(err)
	}
	readLive(t, c, &state)
	var denied struct {
		Code int `json:"code"`
	}
	readLive(t, c, &denied)
	if denied.Code != 401 {
		t.Fatal("expired session retained live access")
	}
}

func TestLiveClosesWhenAPIShutsDown(t *testing.T) {
	a := integrationAPI(t)
	ctx, cancel := context.WithCancel(a.Context)
	defer cancel()
	a.Context = ctx
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var state protocol.Workspace
	readLive(t, c, &state)
	cancel()
	deadline, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if _, _, err := c.Read(deadline); err == nil {
		t.Fatal("API shutdown left a stream open")
	}
}

func TestExpiryAtSnapshotBoundary(t *testing.T) {
	now := time.Now()
	seen := now.Add(-20 * time.Second)
	next := nextExpiry(protocol.Workspace{Hosts: []protocol.Host{{Online: true, LastSeen: &seen}}}, time.Time{}, now)
	if next.IsZero() || !next.After(now) || next.After(now.Add(time.Second)) {
		t.Fatal("missed an online-to-offline boundary during snapshot creation")
	}
}

func TestLiveUsesExactConfiguredBrowserOrigin(t *testing.T) {
	a := &API{Origin: "https://clutchbase.example", AdminToken: strings.Repeat("a", 32)}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// A matching request host alone must not override the configured HTTPS origin.
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/live", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {server.URL}}})
	if err == nil {
		conn.CloseNow()
		t.Fatal("accepted a browser origin outside the configured application")
	}
	if response == nil || response.StatusCode != 403 {
		t.Fatal("wrong configured-origin rejection", err)
	}
}

func TestPresetPersistenceAcknowledgementAndRuntime(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	machineToken := "fixture-machine-token"
	if _, err := a.DB.Exec(ctx, "INSERT INTO machines(id,name,address,token_hash,last_seen) VALUES('presets','Preset host','localhost',$1,now())", digest(machineToken)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, "INSERT INTO hosts(id,name,address,port,token_hash,machine_id,last_seen,observed_state,driver) VALUES('preset-host','Presets','localhost',27015,'','presets',now(),'stopped','managed')"); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler()
	call := func(method, path, token, body string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	path := "/api/hosts/preset-host"
	heartbeat := "/api/machine-agent/presets/heartbeat"
	call("PATCH", path, a.AdminToken, `{"revision":1,"preset":"practice"}`, 409)
	report := protocol.MachineReport{Driver: "managed", SupportsPresets: true, Servers: []protocol.ServerReport{{ID: "preset-host", Report: protocol.Report{State: "stopped", Driver: "managed"}}}}
	send := func() protocol.MachineDesired {
		data, _ := json.Marshal(report)
		raw := call("POST", heartbeat, machineToken, string(data), 200)
		var out protocol.MachineDesired
		json.Unmarshal(raw, &out)
		return out
	}
	send()
	call("PATCH", path, machineToken, `{"revision":1,"preset":"practice"}`, 401)
	call("PATCH", path, a.AdminToken, `{"revision":1,"preset":"practice;quit"}`, 400)
	call("PATCH", path, a.AdminToken, `{"revision":1,"preset":"practice","desiredState":"running"}`, 409)
	var h protocol.Host
	json.Unmarshal(call("PATCH", path, a.AdminToken, `{"revision":1,"preset":"practice"}`, 200), &h)
	if h.Preset == nil || *h.Preset != "practice" || h.PresetApplied || h.Actions.Start {
		t.Fatal("optimistic preset application")
	}
	call("PATCH", path, a.AdminToken, `{"revision":2,"desiredState":"running"}`, 409)
	desired := send()
	if desired.Servers[0].Preset == nil || *desired.Servers[0].Preset != "practice" {
		t.Fatal("canonical preset not sent")
	}
	applied := "practice"
	report.Servers[0].AppliedPreset = &applied
	send()
	call("PATCH", path, a.AdminToken, `{"revision":2,"desiredState":"running"}`, 200)
	call("PATCH", path, a.AdminToken, `{"revision":3,"preset":"match"}`, 409)
	report.Servers[0].State = "running"
	report.Servers[0].AccessStatus = "verified"
	report.Servers[0].ActivePreset = "practice"
	send()
	var state protocol.Workspace
	json.Unmarshal(call("GET", "/api/state", a.AdminToken, "", 200), &state)
	if state.Hosts[0].PresetStatus != "verified" {
		t.Fatal("runtime mode not canonical")
	}
	report.SupportsPresets = false
	desired = send()
	if desired.Servers[0].State != "stopped" || desired.Servers[0].Preset != nil {
		t.Fatal("old agent received a running preset-managed game")
	}
}
