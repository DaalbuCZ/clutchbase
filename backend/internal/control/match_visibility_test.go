package control

import (
	"context"
	"fmt"
	"testing"
)

func TestMatchSnapshotsKeepActionableRoomsAndBoundHistory(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const steam = "76561198154367261"
	const open = "open-room-00000000000001"
	const failed = "failed-room-000000000001"
	const active = "active-room-000000000001"
	if _, err := a.DB.Exec(ctx, `INSERT INTO hosts(id,name,address,port,token_hash,whitelist,active_match_setup_id) VALUES
        ('visible','Visible','localhost',27015,'',ARRAY[$1]::text[],'ACTIVESETUP'),
        ('hidden','Hidden','localhost',27025,'',ARRAY[]::text[],'')`, steam); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,created_at,load_status,load_command_id) VALUES
        ($1,'visible','Open','de_mirage','open',now()-interval '3 days','',''),
        ($2,'visible','Failed','de_mirage','closed',now()-interval '3 days','failed','FAILEDSETUP'),
        ($3,'visible','Active','de_mirage','closed',now()-interval '3 days','complete','ACTIVESETUP')`, open, failed, active); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(ctx, `INSERT INTO match_rooms(id,host_id,title,map,status,created_at)
        SELECT 'history-visible-'||lpad(i::text,6,'0'),'visible','History','de_mirage','closed',now()-i*interval '1 minute' FROM generate_series(1,60) i;
        INSERT INTO match_rooms(id,host_id,title,map,status,created_at)
        SELECT 'history-hidden--'||lpad(i::text,6,'0'),'hidden','Hidden','de_mirage','closed',now()-i*interval '1 second' FROM generate_series(1,60) i`); err != nil {
		t.Fatal(err)
	}
	admin, err := a.matches(ctx)
	if err != nil {
		t.Fatal(err)
	}
	player, err := a.playerMatches(ctx, steam)
	if err != nil {
		t.Fatal(err)
	}
	if len(admin) != 53 || len(player) != 53 {
		t.Fatalf("got admin %d and player %d, want 53 each", len(admin), len(player))
	}
	adminIDs, playerIDs := map[string]bool{}, map[string]bool{}
	for _, room := range admin {
		adminIDs[room.ID] = true
	}
	for _, room := range player {
		playerIDs[room.ID] = true
		if room.HostID != "visible" {
			t.Fatalf("hidden room exposed: %+v", room)
		}
	}
	for _, id := range []string{open, failed, active} {
		if !adminIDs[id] || !playerIDs[id] {
			t.Fatalf("important room %s missing", id)
		}
	}
	for i := 1; i <= 60; i++ {
		id := fmt.Sprintf("history-visible-%06d", i)
		if playerIDs[id] != (i <= 50) {
			t.Fatalf("wrong player history selection for %s", id)
		}
	}
	if adminIDs["history-visible-000001"] {
		t.Fatal("admin history should be bounded across servers")
	}
}
