package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestWhitelistPolicyAndWireFormat(t *testing.T) {
	now := time.Now()
	ids := []string{"76561198154367261"}
	h := Host{Map: "de_mirage", MachineID: "machine", CanWhitelist: true, DesiredState: "stopped", ObservedState: "stopped", LastSeen: &now, Whitelist: ids}
	h.Project(now)
	if h.Actions.Start || !h.Actions.ChangeWhitelist || h.WhitelistApplied {
		t.Fatal("unapplied roster allowed start")
	}
	h.WhitelistHash = WhitelistHash(ids)
	h.Project(now)
	if !h.Actions.Start || !h.WhitelistApplied {
		t.Fatal("matching acknowledgement did not allow start")
	}
	for _, change := range []func(*Host){
		func(h *Host) { h.CanWhitelist = false },
		func(h *Host) { h.LastSeen = nil },
		func(h *Host) { h.WhitelistHash = WhitelistHash([]string{}) },
	} {
		next := h
		change(&next)
		next.Project(now)
		if next.Actions.Start || next.WhitelistApplied {
			t.Fatal("unsafe roster acknowledgement")
		}
	}
	disabled := []string{}
	data, err := json.Marshal(Desired{Whitelist: &disabled})
	if err != nil || !strings.Contains(string(data), `"whitelist":[]`) {
		t.Fatalf("lost explicit disable: %s %v", data, err)
	}
	for _, invalid := range [][]string{{"76561197960265728"}, {"76561202255233024"}, {"76561198154367261;quit"}, {"76561198154367261", "76561198154367261"}, make([]string, 257)} {
		if ValidateWhitelist(invalid) == nil {
			t.Fatal("accepted invalid roster")
		}
	}
}
