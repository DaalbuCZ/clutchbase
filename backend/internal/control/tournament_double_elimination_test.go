package control

import (
	"strconv"
	"testing"
)

func TestDoubleEliminationBracketShape(t *testing.T) {
	for _, count := range []int{3, 4, 5, 8} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			teams := make([]string, count)
			for i := range teams {
				teams[i] = "team-" + strconv.Itoa(i+1)
			}
			matches := doubleEliminationBracket(teams)
			size := 1
			for size < count {
				size *= 2
			}
			if len(matches) != 2*size-1 {
				t.Fatalf("got %d matches, want %d", len(matches), 2*size-1)
			}
			byID := map[string]doubleBracketMatch{}
			for _, match := range matches {
				byID[match.id] = match
			}
			for _, match := range matches {
				if match.winnerMatch != "" {
					target, ok := byID[match.winnerMatch]
					if !ok {
						t.Fatalf("winner route from %s is missing", match.id)
					}
					if match.winnerSlot == 1 && target.sourceA != match.id || match.winnerSlot == 2 && target.sourceB != match.id {
						t.Fatalf("winner slot is inconsistent for %s", match.id)
					}
				}
				if match.loserMatch != "" {
					target, ok := byID[match.loserMatch]
					if !ok {
						t.Fatalf("loser route from %s is missing", match.id)
					}
					if match.loserSlot == 1 && target.sourceA != match.id || match.loserSlot == 2 && target.sourceB != match.id {
						t.Fatalf("loser slot is inconsistent for %s", match.id)
					}
				}
			}
			if matches[0].a != teams[0] || matches[0].b != teams[1] {
				t.Fatalf("initial seeding is not stable: %+v", matches[0])
			}
			if matches[len(matches)-2].stage != "grand_final" || matches[len(matches)-1].stage != "reset_final" {
				t.Fatal("grand final and reset final are missing")
			}
		})
	}
}
