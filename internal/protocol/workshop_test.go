package protocol

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidWorkshopMap(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"workshop/123/de_example", true}, {"workshop/1/a-b_1", true},
		{"workshop/0/a", false}, {"workshop/01/a", false}, {"workshop/1/../a", false},
		{"workshop/1/a;b", false}, {"workshop/1/a\nmap", false}, {"workshop/1/A", false},
		{"workshop/1/a.bsp", false}, {"workshop/1/", false}, {"workshop/123456789012345678901/a", false},
	} {
		if got := ValidWorkshopMap(tc.name); got != tc.valid {
			t.Errorf("%q: got %v", tc.name, got)
		}
	}
}

func TestWorkshopReportLimits(t *testing.T) {
	if !ValidWorkshopMaps(nil) || !ValidWorkshopMaps([]string{}) {
		t.Fatal("empty scan invalid")
	}
	if ValidWorkshopMaps([]string{"workshop/1/map", "workshop/1/map"}) || ValidWorkshopMaps([]string{"de_mirage"}) {
		t.Fatal("invalid inventory accepted")
	}
	maps := make([]string, MaxWorkshopMaps+1)
	if ValidWorkshopMaps(maps) {
		t.Fatal("oversized count accepted")
	}
	for i := range maps[:MaxWorkshopMaps] {
		maps[i] = fmt.Sprintf("workshop/%d/%s", i+1, strings.Repeat("a", 128))
	}
	if ValidWorkshopMaps(maps[:MaxWorkshopMaps]) {
		t.Fatal("oversized bytes accepted")
	}
}

func TestWorkshopProjectionFreshness(t *testing.T) {
	now := time.Now()
	h := Host{Map: "workshop/123/custom", Driver: "managed", LastSeen: &now, WorkshopMapsSeen: &now, WorkshopMaps: []string{"workshop/123/custom"}, DesiredState: "stopped", ObservedState: "stopped"}
	h.Project(now)
	if !h.Actions.Start || !slices.Contains(h.AvailableMaps, h.Map) {
		t.Fatal("fresh map unavailable")
	}
	h.Project(now.Add(20 * time.Second))
	if h.Actions.Start || slices.Contains(h.AvailableMaps, h.Map) {
		t.Fatal("expired map available")
	}
	h.Project(now)
	h.WorkshopMaps = nil
	h.Project(now)
	if h.Actions.Start || len(h.AvailableMaps) != len(Maps) {
		t.Fatal("disappeared map available")
	}
}
