package manager

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

func (m *Manager) RequestMatchSetup(ctx context.Context, name string, setup protocol.MatchSetup) error {
	if !protocol.ValidCommandID(setup.ID) || !protocol.ValidInstalledMap(setup.Map) || setup.PlayersPerSide < 1 || setup.PlayersPerSide > 5 || len(setup.CT) < 1 || len(setup.T) < 1 || len(setup.CT) > setup.PlayersPerSide || len(setup.T) > setup.PlayersPerSide {
		return errors.New("invalid MatchZy match setup")
	}
	if len(setup.Maps) == 0 {
		setup.Maps = []string{setup.Map}
		setup.MapSides = []string{"team1_ct"}
	}
	if (len(setup.Maps) != 1 && len(setup.Maps) != 3 && len(setup.Maps) != 5) || len(setup.MapSides) != len(setup.Maps) || setup.Maps[0] != setup.Map {
		return errors.New("invalid MatchZy series")
	}
	seenMaps := map[string]bool{}
	for i, mapName := range setup.Maps {
		if !protocol.ValidInstalledMap(mapName) || seenMaps[mapName] {
			return errors.New("invalid map in MatchZy series")
		}
		if err := m.CheckInstalledMap(name, mapName); err != nil {
			return err
		}
		seenMaps[mapName] = true
		if setup.MapSides[i] != "team1_ct" && setup.MapSides[i] != "team1_t" {
			return errors.New("invalid MatchZy map side")
		}
	}
	ids := make([]string, 0, len(setup.CT)+len(setup.T))
	for _, side := range [][]protocol.MatchSetupPlayer{setup.CT, setup.T} {
		for _, player := range side {
			ids = append(ids, player.SteamID)
			name := strings.TrimSpace(player.GameName)
			if len(name) < 1 || len([]rune(name)) > 32 || strings.ContainsAny(name, "\r\n\x00") {
				return errors.New("invalid player name in MatchZy match setup")
			}
		}
	}
	if err := protocol.ValidateWhitelist(ids); err != nil {
		return err
	}
	if setup.RequestedAt != 0 {
		age := time.Since(time.UnixMilli(setup.RequestedAt))
		if age > 60*time.Second || age < -5*time.Second {
			return errors.New("MatchZy match setup request is expired or the clocks differ")
		}
	}
	release, err := m.access(name)
	if err != nil {
		return err
	}
	defer release()
	path := filepath.Join(m.accessDir(name), "match-request.json")
	var previous access.MatchSetupRequest
	if access.Read(path, &previous) == nil && previous.ID == setup.ID {
		if len(previous.Maps) == 0 {
			previous.Maps = []string{previous.Map}
			previous.MapSides = []string{"team1_ct"}
		}
		if previous.Map != setup.Map || previous.ManualReady != setup.ManualReady || previous.Overtime != setup.Overtime || previous.PlayersPerSide != setup.PlayersPerSide || previous.FenceRequired != setup.FenceRequired || (setup.RequestedAt != 0 && previous.RequestedAt != setup.RequestedAt) || !slices.Equal(previous.Maps, setup.Maps) || !slices.Equal(previous.MapSides, setup.MapSides) || !slices.Equal(previous.CT, setup.CT) || !slices.Equal(previous.T, setup.T) {
			return errors.New("match setup ID was reused")
		}
		if setup.FenceRequired {
			state := m.Status(ctx, name)
			if state.AccessStatus != "verified" || !state.SupportsMatchFence {
				return errors.New("MatchZy setup fence is unavailable")
			}
		}
		if setup.ManualReady {
			state := m.Status(ctx, name)
			if state.AccessStatus != "verified" || !state.SupportsManualReady {
				return errors.New("manual in-game ready requires an updated Clutchbase Access reporter")
			}
		}
		return nil
	}
	report := m.Status(ctx, name)
	if report.State != "running" || report.AccessStatus != "verified" || !report.CanLoadMatch || report.ActivePreset != "match" || (setup.FenceRequired && !report.SupportsMatchFence) {
		return errors.New("MatchZy match loading is unavailable")
	}
	if len(setup.Maps) > 1 && !report.SupportsMatchSeries {
		return errors.New("BO3/BO5 loading requires the updated Clutchbase Access reporter")
	}
	if setup.ManualReady && !report.SupportsManualReady {
		return errors.New("manual in-game ready requires an updated Clutchbase Access reporter")
	}
	requestedAt := setup.RequestedAt
	if requestedAt == 0 {
		// Compatibility for requests saved before timestamps were introduced.
		requestedAt = time.Now().UnixMilli()
	}
	request := access.MatchSetupRequest{ID: setup.ID, FenceRequired: setup.FenceRequired, ManualReady: setup.ManualReady, Map: setup.Map, Maps: setup.Maps, MapSides: setup.MapSides, Overtime: setup.Overtime, PlayersPerSide: setup.PlayersPerSide, CT: setup.CT, T: setup.T, RequestedAt: requestedAt}
	return access.Write(path, request)
}
