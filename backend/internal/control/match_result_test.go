package control

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestManualMatchResult(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const room = "RESULTROOMABCDEFGHIJKLMNOP"
	const player = "76561198154367261"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,whitelist) VALUES('server','Server','localhost',27015,'',ARRAY[$1]::text[])`, player); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,load_status) VALUES('RESULTROOMABCDEFGHIJKLMNOP','server','Match','de_mirage','closed','complete')`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Handler())
	defer server.Close()
	live := liveDial(t, server.URL+"/api/live", a.AdminToken, nil)
	var initial protocol.Workspace
	readLive(t, live, &initial)
	if len(initial.Matches) != 1 || !initial.Matches[0].CanEditResult || initial.Matches[0].ResultCT != nil {
		t.Fatalf("initial: %+v", initial.Matches)
	}
	call := func(token, body string, want int) {
		t.Helper()
		req := httptest.NewRequest("PATCH", "/api/matches/"+room+"/result", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s: got %d want %d: %s", body, res.Code, want, res.Body.String())
		}
	}
	call("wrong", `{"ct":13,"t":9,"revision":0}`, 401)
	call(a.AdminToken, `{"ct":13,"revision":0}`, 400)
	call(a.AdminToken, `{"ct":100,"t":9,"revision":0}`, 400)
	call(a.AdminToken, `{"ct":-1,"t":9,"revision":0}`, 400)
	call(a.AdminToken, `{"ct":13,"t":9}`, 400)
	call(a.AdminToken, `{"ct":13,"t":9,"clear":true,"revision":0}`, 400)
	call(a.AdminToken, `{"ct":13,"t":9,"revision":0}`, 204)
	var updated protocol.Workspace
	readLive(t, live, &updated)
	result := updated.Matches[0]
	if result.ResultCT == nil || *result.ResultCT != 13 || result.ResultT == nil || *result.ResultT != 9 || result.ResultRevision != 1 {
		t.Fatalf("live result: %+v", result)
	}
	view, err := a.playerMatches(ctx, player)
	if err != nil || len(view) != 1 || view[0].ResultCT == nil || *view[0].ResultCT != 13 {
		t.Fatalf("player result: %+v %v", view, err)
	}
	hidden, err := a.playerMatches(ctx, "76561198154367262")
	if err != nil || len(hidden) != 0 {
		t.Fatalf("unlisted player: %+v %v", hidden, err)
	}
	call(a.AdminToken, `{"ct":11,"t":13,"revision":0}`, 409)
	call(a.AdminToken, `{"ct":11,"t":13,"revision":1}`, 204)
	call(a.AdminToken, `{"clear":true,"revision":2}`, 204)
	rooms, err := a.matches(ctx)
	if err != nil || rooms[0].ResultCT != nil || rooms[0].ResultT != nil || rooms[0].ResultRevision != 3 {
		t.Fatalf("cleared: %+v %v", rooms, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='open' WHERE id=$1`, room); err != nil {
		t.Fatal(err)
	}
	call(a.AdminToken, `{"ct":13,"t":9,"revision":3}`, 409)
	if _, err := a.DB.Exec(ctx, `UPDATE match_rooms SET status='closed',load_status='failed' WHERE id=$1`, room); err != nil {
		t.Fatal(err)
	}
	call(a.AdminToken, `{"ct":13,"t":9,"revision":3}`, 409)
}
