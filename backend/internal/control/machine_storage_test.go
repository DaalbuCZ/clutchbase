package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestMachineStorageReportsAndExpiry(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	const token = "machine-secret"
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest(token)); err != nil {
		t.Fatal(err)
	}
	call := func(report protocol.MachineReport, want int) {
		t.Helper()
		body, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		a.Handler().ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("status %d want %d: %s", res.Code, want, res.Body.String())
		}
	}
	report := protocol.MachineReport{Driver: "managed", Storage: &protocol.MachineStorage{TotalBytes: 1000, AvailableBytes: 300}}
	call(report, 200)
	machines, err := a.machines(ctx)
	if err != nil || len(machines) != 1 || machines[0].Storage == nil || machines[0].Storage.AvailableBytes != 300 {
		t.Fatalf("storage: %+v %v", machines, err)
	}
	report.Storage.AvailableBytes = 1001
	call(report, 400)
	report.Storage.AvailableBytes = -1
	call(report, 400)
	report.Storage.AvailableBytes = 0
	report.Storage.TotalBytes = 0
	call(report, 400)
	report.Storage.TotalBytes = 1000
	report.Driver = "simulated"
	call(report, 400)
	machines, err = a.machines(ctx)
	if err != nil || machines[0].Storage == nil || machines[0].Storage.AvailableBytes != 300 {
		t.Fatalf("invalid report changed state: %+v %v", machines, err)
	}
	if _, err := a.DB.Exec(ctx, `UPDATE machines SET last_seen=now()-interval '30 seconds' WHERE id='machine'`); err != nil {
		t.Fatal(err)
	}
	machines, err = a.machines(ctx)
	if err != nil || machines[0].Online || machines[0].Storage != nil {
		t.Fatalf("stale capacity exposed: %+v %v", machines, err)
	}
	call(protocol.MachineReport{Driver: "managed"}, 200)
	machines, err = a.machines(ctx)
	if err != nil || !machines[0].Online || machines[0].Storage != nil {
		t.Fatalf("old agent must clear capacity: %+v %v", machines, err)
	}
}
