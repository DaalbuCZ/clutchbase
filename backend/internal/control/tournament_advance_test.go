package control

import "testing"

func TestTournamentVetoOrders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bestOf int
		picks  []bool
		turns  []string
	}{
		{"BO1 preserves its existing ban turns", 1, []bool{false, false, false, false, false, false}, []string{"a", "a", "b", "b", "b", "a", "b"}},
		{"BO3 picks two maps", 3, []bool{false, false, true, true, false, false}, []string{"a", "b", "a", "b", "a", "b", "b"}},
		{"BO5 picks four maps", 5, []bool{false, false, true, true, true, true}, []string{"a", "b", "a", "b", "a", "b", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for step := 0; step < 6; step++ {
				if got := tournamentVetoPick(tc.bestOf, step); got != tc.picks[step] {
					t.Fatalf("step %d pick=%v, want %v", step, got, tc.picks[step])
				}
				if got := tournamentVetoTurn(tc.bestOf, step+1, "a", "b"); got != tc.turns[step+1] {
					t.Fatalf("next step %d turn=%s, want %s", step+1, got, tc.turns[step+1])
				}
			}
		})
	}
}

func TestReportedTournamentWinner(t *testing.T) {
	tests := []struct {
		name       string
		teamBSide  string
		ct, scoreT int
		winner     string
	}{
		{name: "team B starts CT and wins", teamBSide: "ct", ct: 13, scoreT: 8, winner: "team-b"},
		{name: "team A starts CT and wins", teamBSide: "t", ct: 13, scoreT: 8, winner: "team-a"},
		{name: "team A starts CT and team B wins", teamBSide: "t", ct: 7, scoreT: 13, winner: "team-b"},
		{name: "draw waits for a manager", teamBSide: "ct", ct: 12, scoreT: 12},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			winner, decided := reportedTournamentWinner("team-a", "team-b", test.teamBSide, test.ct, test.scoreT)
			if test.winner == "" {
				if decided || winner != "" {
					t.Fatalf("draw selected a winner: %q, %v", winner, decided)
				}
			} else if !decided || winner != test.winner {
				t.Fatalf("got %q, %v; want %q", winner, decided, test.winner)
			}
		})
	}
}

func TestReportedTournamentSeriesWinner(t *testing.T) {
	two, one, three, zero, four := 2, 1, 3, 0, 4
	for _, tc := range []struct {
		name          string
		teamBSide     string
		bestOf        int
		first, second *int
		want          string
	}{
		{"BO3 team A wins when team B started T", "t", 3, &two, &one, "team-a"},
		{"BO3 team B wins when team B started CT", "ct", 3, &two, &one, "team-b"},
		{"BO5 team B roster wins 3-0", "t", 5, &zero, &three, "team-b"},
		{"tied series waits", "ct", 3, &one, &one, ""},
		{"premature series waits", "ct", 5, &two, &one, ""},
		{"out of range series waits", "ct", 3, &four, &zero, ""},
		{"missing result waits", "ct", 3, nil, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, decided := reportedTournamentSeriesWinner("team-a", "team-b", tc.teamBSide, tc.bestOf, tc.first, tc.second)
			if got != tc.want || decided != (tc.want != "") {
				t.Fatalf("got %q decided=%v; want %q", got, decided, tc.want)
			}
		})
	}
}
