package manager

import (
	"testing"

	"clutchbase/internal/protocol"
)

func TestMatchControlRequiresSetup(t *testing.T) {
	for _, action := range []string{"pause", "resume", "end"} {
		if validMatchControl(protocol.MatchControl{Action: action}) {
			t.Fatalf("%s accepted without a setup", action)
		}
		if !validMatchControl(protocol.MatchControl{Action: action, SetupID: "ABCDEFGHIJKLMNOPQRSTUVWX23"}) {
			t.Fatalf("%s rejected a valid setup", action)
		}
	}
}
