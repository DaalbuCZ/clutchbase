package control

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestPlayerDirectoryAdminOnlyAndLive(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const first = "76561198154367261"
	const second = "76561198154367262"
	token := rand.Text() + rand.Text()
	if _, err := a.DB.Exec(ctx, `INSERT INTO players(steam_id,game_name) VALUES($1,'First'),($2,'Second')`, first, second); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO player_sessions(token_hash,steam_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, digest(token), first); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	admin := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var state protocol.Workspace
	readLive(t, admin, &state)
	if len(state.Players) != 2 || state.Players[0].SteamID != first || state.Players[1].GameName != "Second" {
		t.Fatalf("directory: %+v", state.Players)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE players SET game_name='Renamed' WHERE steam_id=$1`, second); err != nil {
		t.Fatal(err)
	}
	readLive(t, admin, &state)
	if len(state.Players) != 2 || state.Players[1].GameName != "Renamed" {
		t.Fatalf("directory update: %+v", state.Players)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: playerCookie, Value: token})
	res := httptest.NewRecorder()
	a.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("player accessed admin state: %d", res.Code)
	}
	player := liveDial(t, server.URL+"/api/live?view=player", "", http.Header{"Cookie": {playerCookie + "=" + token}})
	var view map[string]json.RawMessage
	readLive(t, player, &view)
	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := view["players"]; ok || strings.Contains(string(payload), second) {
		t.Fatal("player snapshot exposed the directory")
	}
}
