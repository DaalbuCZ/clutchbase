package control

import (
	"clutchbase/internal/protocol"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type steamTransport func(*http.Request) (*http.Response, error)

func (f steamTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func steamAssertion(returnTo string) url.Values {
	id := "https://steamcommunity.com/openid/id/76561198154367261"
	return url.Values{
		"openid.ns": {openIDNamespace}, "openid.mode": {"id_res"}, "openid.op_endpoint": {steamEndpoint},
		"openid.claimed_id": {id}, "openid.identity": {id}, "openid.return_to": {returnTo},
		"openid.response_nonce": {time.Now().UTC().Format("2006-01-02T15:04:05Z") + rand.Text()},
		"openid.assoc_handle":   {"fixture"}, "openid.sig": {"fixture"},
		"openid.signed": {"op_endpoint,claimed_id,identity,return_to,response_nonce,assoc_handle"},
	}
}

func verifyingSteamClient(t *testing.T, response string) *http.Client {
	t.Helper()
	return &http.Client{Transport: steamTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != steamEndpoint || r.Method != "POST" {
			t.Error("verification sent to an untrusted endpoint")
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("openid.mode") != "check_authentication" {
			t.Error("not using Steam direct verification")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response)), Header: http.Header{}}, nil
	})}
}

func TestSteamAssertionValidation(t *testing.T) {
	a := &API{Origin: "https://clutchbase.example", SteamClient: verifyingSteamClient(t, "ns:"+openIDNamespace+"\nis_valid:true\n")}
	target := a.Origin + "/api/auth/steam/callback?state=fixture"
	if id, err := a.verifySteam(context.Background(), steamAssertion(target), target); err != nil || id != "76561198154367261" {
		t.Fatal("valid assertion rejected", id, err)
	}
	for name, change := range map[string]func(url.Values){
		"foreign endpoint":  func(v url.Values) { v.Set("openid.op_endpoint", "https://attacker.example") },
		"foreign identity":  func(v url.Values) { v.Set("openid.identity", "https://attacker.example/76561198154367261") },
		"wrong return URL":  func(v url.Values) { v.Set("openid.return_to", "https://attacker.example") },
		"unsigned identity": func(v url.Values) { v.Set("openid.signed", "op_endpoint,return_to,response_nonce,assoc_handle") },
		"expired nonce": func(v url.Values) {
			v.Set("openid.response_nonce", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)+"old")
		},
		"future nonce": func(v url.Values) {
			v.Set("openid.response_nonce", time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+"future")
		},
		"invalid ID": func(v url.Values) {
			v.Set("openid.claimed_id", "https://steamcommunity.com/openid/id/0")
			v.Set("openid.identity", v.Get("openid.claimed_id"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := steamAssertion(target)
			change(v)
			if _, err := a.verifySteam(context.Background(), v, target); err == nil {
				t.Fatal("accepted invalid assertion")
			}
		})
	}
	a.SteamClient = verifyingSteamClient(t, "is_valid:false\n")
	if _, err := a.verifySteam(context.Background(), steamAssertion(target), target); err == nil {
		t.Fatal("accepted rejected provider response")
	}
	a.SteamClient = verifyingSteamClient(t, "is_valid:true\nis_valid:true\n")
	if _, err := a.verifySteam(context.Background(), steamAssertion(target), target); err == nil {
		t.Fatal("accepted ambiguous provider response")
	}
	a.Origin = "http://public.example"
	if a.origin() != "" {
		t.Fatal("accepted public plaintext Steam origin")
	}
	a.Origin = "https://clutchbase.example/path"
	if a.origin() != "" {
		t.Fatal("accepted an ambiguous application origin")
	}
}

func TestSteamLoginSessionAndPlayerPermissions(t *testing.T) {
	a := integrationAPI(t)
	a.SteamClient = verifyingSteamClient(t, "is_valid:true\n")
	handler := a.Handler()
	call := func(method, path string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	start := call("GET", "/api/auth/steam", nil, "")
	if start.Code != 302 {
		t.Fatal("Steam login did not redirect", start.Code, start.Body.String())
	}
	provider, err := url.Parse(start.Header().Get("Location"))
	if err != nil || provider.Scheme+"://"+provider.Host+provider.Path != steamEndpoint {
		t.Fatal("wrong provider redirect")
	}
	cookies := start.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != loginCookie || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe login cookie")
	}
	stateCookie := cookies[0]
	var stored string
	if err := a.DB.QueryRow(context.Background(), "SELECT state_hash FROM steam_logins").Scan(&stored); err != nil || stored == stateCookie.Value || stored != digest(stateCookie.Value) {
		t.Fatal("login state was not hashed", err)
	}
	returnTo := provider.Query().Get("openid.return_to")
	callback, err := url.Parse(returnTo)
	if err != nil {
		t.Fatal(err)
	}
	values := steamAssertion(returnTo)
	values.Set("state", callback.Query().Get("state"))
	callbackURL := "/api/auth/steam/callback?" + values.Encode()
	if response := call("GET", callbackURL, nil, ""); response.Code != 400 {
		t.Fatal("accepted missing login-state cookie")
	}
	duplicate := values.Encode() + "&openid.identity=evil"
	if response := call("GET", "/api/auth/steam/callback?"+duplicate, stateCookie, ""); response.Code != 400 {
		t.Fatal("accepted duplicate assertion field")
	}
	response := call("GET", callbackURL, stateCookie, "")
	if response.Code != 303 || response.Header().Get("Location") != a.Origin+"/play" {
		t.Fatal("valid login failed", response.Code, response.Body.String())
	}
	var session *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == playerCookie {
			session = cookie
		}
	}
	if session == nil || !session.HttpOnly || session.Path != "/api" || session.MaxAge != 86400 {
		t.Fatal("missing private player session")
	}
	if response := call("GET", callbackURL, stateCookie, ""); response.Code != 400 {
		t.Fatal("replayed login callback")
	}
	if response := call("GET", "/api/player/state", session, ""); response.Code != 200 || !strings.Contains(response.Body.String(), `"servers":[]`) {
		t.Fatal("new player state failed")
	}
	if response := call("GET", "/api/state", session, ""); response.Code != 401 {
		t.Fatal("Steam session granted manager access")
	}
	if response := call("POST", "/api/machines", session, ""); response.Code != 401 {
		t.Fatal("Steam player mutated management data")
	}
	if response := call("POST", "/api/player/logout", session, "https://attacker.example"); response.Code != 403 {
		t.Fatal("cross-site sign out accepted")
	}
	if response := call("GET", "/api/player/state", session, ""); response.Code != 200 {
		t.Fatal("rejected logout changed session")
	}
	if response := call("POST", "/api/player/logout", session, a.Origin); response.Code != 204 {
		t.Fatal("logout failed", response.Body.String())
	}
	if response := call("GET", "/api/player/state", session, ""); response.Code != 401 {
		t.Fatal("logged-out cookie retained access")
	}
	a.Origin = "https://clutchbase.example"
	w := httptest.NewRecorder()
	a.setPlayerCookie(w, playerCookie, "fixture", 86400)
	if !w.Result().Cookies()[0].Secure {
		t.Fatal("HTTPS session cookie lacks Secure")
	}
}

func TestPlayerReturnPath(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", "/play"},
		{"#teams", "/play#teams"},
		{"#schedule", "/play#schedule"},
		{"#matches?room=friday-1", "/play#matches?room=friday-1"},
		{"#tournaments?tournament=cup_2", "/play#tournaments?tournament=cup_2"},
		{"#stats", "/play#stats"},
		{"https://attacker.example", "/play"},
		{"//attacker.example", "/play"},
		{"/manage", "/play"},
		{"#settings", "/play"},
		{"#teams?next=https://attacker.example", "/play"},
		{"#teams?invite=abcdefghijklmnopqrst", "/play"},
		{"#teams?invite=abcdefghijklmnopqrst&next=bad", "/play"},
		{"#matches?room=one&room=two", "/play"},
		{"#access?server=abc-123", "/play#access?server=abc-123"},
		{"#access?server=abc&next=bad", "/play"},
		{"#access?server=%0d%0aLocation:evil", "/play"},
		{"#matches?room=one&next=two", "/play"},
		{"#matches?room=", "/play"},
		{"#matches?room=%", "/play"},
		{"#matches?room=%0d%0aLocation:evil", "/play"},
		{"#tournaments?tournament=" + strings.Repeat("a", 129), "/play"},
		{"#matches?room=" + strings.Repeat("a", 513), "/play"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := playerReturnPath(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSteamLoginReturnDestination(t *testing.T) {
	a := integrationAPI(t)
	a.SteamClient = verifyingSteamClient(t, "is_valid:true\n")
	handler := a.Handler()
	for _, tc := range []struct {
		name, query, want string
		expired           bool
	}{
		{"room", url.Values{"returnTo": {"#matches?room=friday"}}.Encode(), "/play#matches?room=friday", false},
		{"access", url.Values{"returnTo": {"#access?server=friday"}}.Encode(), "/play#access?server=friday", false},
		{"tournament", url.Values{"returnTo": {"#tournaments?tournament=cup"}}.Encode(), "/play#tournaments?tournament=cup", false},
		{"external", url.Values{"returnTo": {"https://attacker.example"}}.Encode(), "/play", false},
		{"duplicate", url.Values{"returnTo": {"#teams", "#stats"}}.Encode(), "/play", false},
		{"expired", url.Values{"returnTo": {"#teams"}}.Encode(), "/play#teams", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := httptest.NewRecorder()
			handler.ServeHTTP(start, httptest.NewRequest("GET", "/api/auth/steam?"+tc.query, nil))
			if start.Code != http.StatusFound {
				t.Fatal("login did not start", start.Code)
			}
			provider, err := url.Parse(start.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			cookie := start.Result().Cookies()[0]
			var stored string
			if err := a.DB.QueryRow(context.Background(), "SELECT return_path FROM steam_logins WHERE state_hash=$1", digest(cookie.Value)).Scan(&stored); err != nil || stored != tc.want {
				t.Fatal("destination not bound to login state", stored, err)
			}
			if tc.expired {
				if _, err := a.DB.Exec(context.Background(), "UPDATE steam_logins SET expires_at=now()-interval '1 second' WHERE state_hash=$1", digest(cookie.Value)); err != nil {
					t.Fatal(err)
				}
			}
			values := steamAssertion(provider.Query().Get("openid.return_to"))
			values.Set("state", cookie.Value)
			// A callback query cannot replace the destination saved at sign-in start.
			values.Set("returnTo", "https://attacker.example")
			callback := httptest.NewRequest("GET", "/api/auth/steam/callback?"+values.Encode(), nil)
			callback.AddCookie(cookie)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, callback)
			if tc.expired {
				if response.Code != 400 || response.Header().Get("Location") != "" {
					t.Fatal("expired login redirected", response.Code)
				}
				return
			}
			if response.Code != 303 || response.Header().Get("Location") != a.Origin+tc.want {
				t.Fatal("wrong login destination", response.Code, response.Header().Get("Location"))
			}
			replay := httptest.NewRecorder()
			handler.ServeHTTP(replay, callback)
			if replay.Code != 400 {
				t.Fatal("login destination was reusable", replay.Code)
			}
		})
	}
}

func TestPlayerSimulationStaysLabelledAndNonJoinable(t *testing.T) {
	id := "76561198154367261"
	state := protocol.Workspace{Hosts: []protocol.Host{{ID: "simulation", Driver: "simulated", Whitelist: []string{id}, Status: "running", Online: true, WhitelistApplied: true}}}
	view := projectPlayer(state, id)
	if len(view.Servers) != 1 || !view.Servers[0].Simulated || view.Servers[0].CanJoin {
		t.Fatal("simulation was presented as a real joinable game")
	}
}

func TestPlayerJoinReasonUsesAuthoritativeServerState(t *testing.T) {
	const steamID = "76561198154367261"
	preset := "match"
	ready := protocol.Host{ID: "server", Driver: "managed", Whitelist: []string{steamID}, Status: "running", Online: true, WhitelistApplied: true, AccessStatus: "verified"}
	for _, tc := range []struct {
		name   string
		change func(*protocol.Host)
		want   string
	}{
		{"ready", func(*protocol.Host) {}, ""},
		{"offline", func(h *protocol.Host) { h.Online = false; h.Status = "offline" }, "Server agent is offline."},
		{"starting", func(h *protocol.Host) { h.Status = "starting" }, "CS2 is starting."},
		{"roster pending", func(h *protocol.Host) { h.WhitelistApplied = false }, "SteamID roster is waiting for host confirmation."},
		{"access pending", func(h *protocol.Host) { h.AccessStatus = "pending" }, "SteamID access verification is pending."},
		{"access blocked", func(h *protocol.Host) { h.AccessStatus = "blocked" }, "SteamID access verification failed; ask the host."},
		{"preset unapplied", func(h *protocol.Host) { h.Preset = &preset; h.PresetApplied = false }, "Startup preset is waiting for host confirmation."},
		{"preset mismatch", func(h *protocol.Host) { h.Preset = &preset; h.PresetApplied = true; h.PresetStatus = "mismatch" }, "Startup preset is not verified."},
		{"simulation", func(h *protocol.Host) { h.Driver = "simulated" }, "Simulation does not accept players."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := ready
			tc.change(&host)
			servers := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steamID).Servers
			if len(servers) != 1 || servers[0].JoinBlocker != tc.want || servers[0].CanJoin != (tc.want == "") {
				t.Fatalf("join state=%+v, want blocker %q", servers, tc.want)
			}
		})
	}
}

func TestPlayerSeesObservedMapRatherThanStartupMap(t *testing.T) {
	id := "76561198154367261"
	host := protocol.Host{Driver: "managed", Whitelist: []string{id}, Status: "running", Online: true, DesiredState: "running", ObservedState: "running", Map: "de_mirage", ActiveMap: "de_nuke", CanObserveMap: true}
	view := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, id)
	if len(view.Servers) != 1 || view.Servers[0].Map != "de_nuke" {
		t.Fatalf("observed map not shown: %+v", view.Servers)
	}
	host.CanObserveMap = false
	view = projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, id)
	if view.Servers[0].Map != "" {
		t.Fatalf("unverified map presented as current: %+v", view.Servers[0])
	}
	host.DesiredState = "stopped"
	host.ObservedState = "stopped"
	host.Status = "stopped"
	view = projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, id)
	if view.Servers[0].Map != "de_mirage" {
		t.Fatalf("stopped server should show startup map: %+v", view.Servers[0])
	}
}

func TestPlayerSeesManualReadySupportOnlyFromCurrentManagedHost(t *testing.T) {
	const steamID = "76561198154367261"
	now := time.Now()
	host := protocol.Host{ID: "server", Driver: "managed", Whitelist: []string{steamID}, LastSeen: &now, SupportsManualReady: true}
	host.Project(now)
	servers := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steamID).Servers
	if len(servers) != 1 || !servers[0].SupportsManualReady {
		t.Fatalf("missing current manual-ready capability: %+v", servers)
	}
	host.Project(now.Add(21 * time.Second))
	servers = projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steamID).Servers
	if len(servers) != 1 || servers[0].SupportsManualReady {
		t.Fatalf("stale manual-ready capability was exposed: %+v", servers)
	}
	host.Driver = "simulated"
	host.Project(now)
	servers = projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steamID).Servers
	if len(servers) != 1 || servers[0].SupportsManualReady {
		t.Fatalf("simulated host advertised manual-ready support: %+v", servers)
	}
}

func TestPlayerReceivesBuildStatusWithoutBuildIdentifiers(t *testing.T) {
	const steam = "76561198154367261"
	now := time.Now()
	host := protocol.Host{ID: "server", Driver: "managed", Whitelist: []string{steam}, DesiredState: "running", ObservedState: "running", LastSeen: &now, AccessStatus: "verified", GameBuildInstalled: "123", GameBuildLatest: "456", GameBuildCheckedAt: &now}
	host.Project(now)
	view := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steam)
	if len(view.Servers) != 1 || view.Servers[0].GameBuildStatus != "outdated" {
		t.Fatalf("missing outdated status: %+v", view.Servers)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"gameBuildInstalled"`) || strings.Contains(string(data), `"gameBuildLatest"`) {
		t.Fatalf("build identifiers exposed: %s", data)
	}
	host.GameBuildLatest = "123"
	host.Project(now)
	if got := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steam).Servers[0].GameBuildStatus; got != "current" {
		t.Fatalf("current build: %q", got)
	}
	host.Project(now.Add(2 * time.Hour))
	if got := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{host}}, steam).Servers[0].GameBuildStatus; got != "unknown" {
		t.Fatalf("stale build: %q", got)
	}
}

func TestBuildStatusExpirySchedulesLiveRefresh(t *testing.T) {
	now := time.Now()
	checked := now.Add(-2*time.Hour + time.Second)
	seen := now
	host := protocol.Host{LastSeen: &seen, GameBuildCheckedAt: &checked, GameBuildStatus: "outdated", Online: true}
	if got := nextExpiry(protocol.Workspace{Hosts: []protocol.Host{host}}, time.Time{}, now); !got.Equal(now.Add(time.Second)) {
		t.Fatalf("build expiration: %v", got)
	}
}

func TestPlayerBuildStatusExpiresOnLiveStream(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	token := strings.Repeat("p", 52)
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id) VALUES('76561198154367261'); INSERT INTO hosts(id,name,address,port,token_hash,driver,desired_state,observed_state,last_seen,whitelist,game_build_installed,game_build_latest,game_build_checked_at) VALUES('host','Host','localhost',27015,'','managed','running','running',now(),ARRAY['76561198154367261'],'123','456',now()-interval '2 hours'+interval '2 seconds')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), steam); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	c := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + token}, "Origin": {a.Origin}})
	defer c.CloseNow()
	var view playerWorkspace
	readLive(t, c, &view)
	if len(view.Servers) != 1 || view.Servers[0].GameBuildStatus != "outdated" {
		t.Fatalf("initial build status %+v", view.Servers)
	}
	readLive(t, c, &view)
	if len(view.Servers) != 1 || view.Servers[0].GameBuildStatus != "unknown" {
		t.Fatalf("expired build status %+v", view.Servers)
	}
}
