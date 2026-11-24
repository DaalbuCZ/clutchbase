package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestWorkshopDiscoveryAuthorization(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := a.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1),('other','Other','other.local','unused')`, digest("secret"))
	exec(`INSERT INTO hosts(id,name,address,port,token_hash,machine_id) VALUES('host','Host','localhost',27015,'','machine'),('sibling','Sibling','localhost',27025,'','machine'),('foreign','Foreign','localhost',27035,'','other')`)
	call := func(method, path, token string, body any, want int) []byte {
		t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, strings.NewReader(string(data)))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: %d want %d %s", method, path, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	maps := []string{"workshop/123/de_custom"}
	report := protocol.MachineReport{Driver: "managed", Servers: []protocol.ServerReport{{ID: "host", Report: protocol.Report{Driver: "managed", State: "stopped", WorkshopMaps: &maps}}}}
	heartbeat := func(want int) []byte {
		return call("POST", "/api/machine-agent/machine/heartbeat", "secret", report, want)
	}
	host := func(id string) protocol.Host {
		t.Helper()
		h, err := scan(a.DB.QueryRow(ctx, "SELECT "+columns+" FROM hosts WHERE id=$1", id))
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	heartbeat(200)
	h := host("host")
	if !hostHasMap(h, maps[0]) || hostHasMap(host("sibling"), maps[0]) {
		t.Fatal("inventory not scoped to owner")
	}
	h.Whitelist = []string{"76561198154367261"}
	player := projectPlayer(protocol.Workspace{Hosts: []protocol.Host{h}}, h.Whitelist[0])
	if len(player.Servers) != 1 || !slices.Contains(player.Servers[0].AvailableMaps, maps[0]) {
		t.Fatalf("player inventory: %+v", player)
	}
	call("PATCH", "/api/hosts/host", a.AdminToken, map[string]any{"map": maps[0], "revision": h.Revision}, 200)
	h = host("host")
	call("PATCH", "/api/hosts/host", a.AdminToken, map[string]any{"map": "workshop/999/other", "revision": h.Revision}, 409)
	call("PATCH", "/api/hosts/host", a.AdminToken, map[string]any{"map": "de_nuke", "revision": h.Revision - 1}, 409)
	exec(`UPDATE hosts SET workshop_maps_seen=now()-interval '21 seconds' WHERE id='host'`)
	if host("host").Actions.Start || hostHasMap(host("host"), maps[0]) {
		t.Fatal("stale report authorized start")
	}
	call("PATCH", "/api/hosts/host", a.AdminToken, map[string]any{"desiredState": "running", "revision": h.Revision}, 409)
	heartbeat(200)
	// Malformed and duplicate reports roll back without replacing valid discovery.
	duplicate := []string{maps[0], maps[0]}
	report.Servers[0].WorkshopMaps = &duplicate
	heartbeat(400)
	if !hostHasMap(host("host"), maps[0]) {
		t.Fatal("invalid report changed inventory")
	}
	report.Servers[0].WorkshopMaps = &maps
	report.Servers = append(report.Servers, protocol.ServerReport{ID: "foreign", Report: protocol.Report{Driver: "managed", State: "stopped", WorkshopMaps: &maps}})
	heartbeat(400)
	if !hostHasMap(host("host"), maps[0]) {
		t.Fatal("foreign report did not roll back")
	}
	report.Servers = report.Servers[:1]
	report.Servers[0].WorkshopMaps = nil
	exec(`UPDATE hosts SET desired_state='running' WHERE id='host'`)
	var fenced protocol.MachineDesired
	if err := json.Unmarshal(heartbeat(200), &fenced); err != nil {
		t.Fatal(err)
	}
	for _, desired := range fenced.Servers {
		if desired.ID == "host" && (desired.State != "stopped" || !protocol.ValidMap(desired.Map)) {
			t.Fatalf("unavailable startup issued: %+v", desired)
		}
	}
	if host("host").DesiredState != "running" {
		t.Fatal("issuance rewrote administrator intent")
	}
	if hostHasMap(host("host"), maps[0]) {
		t.Fatal("missing scan did not invalidate inventory")
	}
	report.Servers[0].WorkshopMaps = &maps
	heartbeat(200)
	report.Servers = nil
	heartbeat(200)
	if hostHasMap(host("host"), maps[0]) {
		t.Fatal("omitted instance did not invalidate inventory")
	}
	// Standalone rooms may select only fresh maps belonging to this host.
	report.Servers = []protocol.ServerReport{{ID: "host", Report: protocol.Report{Driver: "managed", State: "running", ActiveMap: "de_mirage", WorkshopMaps: &maps}}}
	heartbeat(200)
	exec(`UPDATE machines SET supports_map_observation=true WHERE id='machine'; UPDATE hosts SET desired_state='running',access_status='verified',preset='match',applied_preset='match',active_preset='match',whitelist=ARRAY['76561198154367261'],can_load_match=true WHERE id='host'`)
	data := call("POST", "/api/matches", a.AdminToken, map[string]any{"hostId": "host", "title": "Friends", "map": maps[0], "playersPerSide": 1}, 201)
	var room protocol.Match
	if err := json.Unmarshal(data, &room); err != nil {
		t.Fatal(err)
	}
	call("PATCH", "/api/matches/"+room.ID+"/settings", a.AdminToken, map[string]any{"title": "Friends", "map": maps[0], "overtime": false, "playersPerSide": 1, "settingsRevision": 1}, 200)
	call("POST", "/api/matches/"+room.ID+"/load", a.AdminToken, nil, 409)
	exec(`UPDATE hosts SET workshop_maps_seen=now()-interval '21 seconds' WHERE id='host'`)
	call("POST", "/api/matches/"+room.ID+"/load", a.AdminToken, nil, 409)
	call("PATCH", "/api/matches/"+room.ID+"/settings", a.AdminToken, map[string]any{"title": "Friends", "map": maps[0], "overtime": false, "playersPerSide": 1, "settingsRevision": 1}, 409)
	setup := protocol.MatchSetup{ID: "ABCDEFGHIJKLMNOPQRSTUVWX23", Map: maps[0], Maps: []string{maps[0]}, PlayersPerSide: 1, CT: []protocol.MatchSetupPlayer{{SteamID: "76561198154367261", GameName: "CT"}}, T: []protocol.MatchSetupPlayer{{SteamID: "76561198154367262", GameName: "T"}}}
	payload, err := json.Marshal(setup)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE match_rooms SET load_status='pending',load_command_id=$2,load_payload=$3::jsonb,load_requested_at=now() WHERE id=$1`, room.ID, setup.ID, string(payload))
	report.SupportsMatchSetup = true
	report.SupportsMapObservation = true
	report.SupportsPresets = true
	report.SupportsWhitelist = true
	report.Servers[0].WorkshopMaps = nil
	var issued protocol.MachineDesired
	if err := json.Unmarshal(heartbeat(200), &issued); err != nil {
		t.Fatal(err)
	}
	for _, desired := range issued.Servers {
		if desired.MatchSetup != nil {
			t.Fatal("unavailable match setup issued")
		}
	}
	var status, message string
	if err := a.DB.QueryRow(ctx, `SELECT load_status,load_error FROM match_rooms WHERE id=$1`, room.ID).Scan(&status, &message); err != nil || status != "failed" || message != "Map is no longer available on this server" {
		t.Fatalf("missing map did not fail pending setup: %s %s %v", status, message, err)
	}
}

func TestWorkshopReportDriverBoundary(t *testing.T) {
	empty := []string{}
	for _, driver := range []string{"simulated", "systemd", "managed"} {
		r := protocol.Report{Driver: driver, State: "stopped", WorkshopMaps: &empty}
		if validReport(r) != (driver == "managed") {
			t.Fatalf("discovery accepted for %s", driver)
		}
		r.WorkshopMaps = nil
		if !validReport(r) {
			t.Fatalf("older %s report rejected", driver)
		}
	}
}
