package control

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"clutchbase/internal/protocol"
)

func TestMachineResourcesPersistValidateAndExpire(t *testing.T) {
	a := integrationAPI(t)
	ctx := context.Background()
	if _, err := a.DB.Exec(ctx, `INSERT INTO machines(id,name,address,token_hash) VALUES('machine','Machine','localhost',$1)`, digest("agent-token")); err != nil {
		t.Fatal(err)
	}
	call := func(report protocol.MachineReport, want int) {
		t.Helper()
		body, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/api/machine-agent/machine/heartbeat", strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer agent-token")
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	cpu := 25.5
	report := protocol.MachineReport{Driver: "managed", Resources: &protocol.MachineResources{CPUPercent: &cpu, MemoryTotalBytes: 8192, MemoryAvailableBytes: 2048}}
	call(report, 200)
	machines, err := a.machines(ctx)
	if err != nil || len(machines) != 1 || machines[0].Resources == nil || *machines[0].Resources.CPUPercent != 25.5 || machines[0].Resources.MemoryAvailableBytes != 2048 {
		t.Fatalf("missing resource projection: %+v %v", machines, err)
	}
	cpu = 101
	call(report, 400)
	cpu = 25.5
	report.Resources.MemoryAvailableBytes = 8193
	call(report, 400)
	report.Resources.MemoryAvailableBytes = -1
	call(report, 400)
	report.Resources.MemoryAvailableBytes = 2048
	report.Driver = "simulated"
	call(report, 400)
	machines, err = a.machines(ctx)
	if err != nil || *machines[0].Resources.CPUPercent != 25.5 || machines[0].Resources.MemoryAvailableBytes != 2048 {
		t.Fatal("invalid report changed stored resources")
	}
	if _, err = a.DB.Exec(ctx, `UPDATE machines SET last_seen=now()-interval '30 seconds' WHERE id='machine'`); err != nil {
		t.Fatal(err)
	}
	machines, err = a.machines(ctx)
	if err != nil || machines[0].Resources != nil || machines[0].Online {
		t.Fatal("offline machine retained live resources")
	}
	report.Driver = "managed"
	report.Resources.CPUPercent = nil
	call(report, 200)
	machines, err = a.machines(ctx)
	if err != nil || machines[0].Resources == nil || machines[0].Resources.CPUPercent != nil {
		t.Fatal("unknown CPU became zero")
	}
	report.Resources = nil
	call(report, 200)
	machines, err = a.machines(ctx)
	if err != nil || machines[0].Resources != nil {
		t.Fatal("old agent heartbeat retained obsolete metrics")
	}
}
