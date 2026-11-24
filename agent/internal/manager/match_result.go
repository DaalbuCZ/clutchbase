package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"clutchbase/agent/internal/access"
	"clutchbase/internal/protocol"
)

type resultFile struct {
	protocol.MatchResult
	ScoreCT    *int `json:"scoreCt"`
	ScoreT     *int `json:"scoreT"`
	Team1Score *int `json:"team1Score"`
	Team2Score *int `json:"team2Score"`
}

func readMatchResult(path string) (protocol.MatchResult, error) {
	var file resultFile
	if err := access.Read(path, &file); err != nil {
		return protocol.MatchResult{}, err
	}
	result := file.MatchResult
	if result.Team1Score == nil {
		result.Team1Score = file.Team1Score
	}
	if result.Team2Score == nil {
		result.Team2Score = file.Team2Score
	}
	if result.CT == -1 && result.T == -1 && file.ScoreCT != nil && file.ScoreT != nil && len(result.Players) > 0 {
		result.CT, result.T = *file.ScoreCT, *file.ScoreT
	} else if file.ScoreCT != nil || file.ScoreT != nil || len(result.Players) > 0 {
		return protocol.MatchResult{}, errors.New("invalid match result format")
	}
	if !result.Valid() {
		return protocol.MatchResult{}, errors.New("invalid match result")
	}
	return result, nil
}

func (m *Manager) PendingMatchResult(name string, supportsStats bool) *protocol.MatchResult {
	if _, err := m.Dir(name); err != nil {
		return nil
	}
	dir := m.accessDir(name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		id, ok := strings.CutPrefix(entry.Name(), "result-finished-")
		if !ok || !strings.HasSuffix(id, ".json") {
			continue
		}
		id = strings.TrimSuffix(id, ".json")
		if !protocol.ValidCommandID(id) {
			continue
		}
		result, err := readMatchResult(filepath.Join(dir, entry.Name()))
		if err != nil || result.SetupID != id {
			continue
		}
		if !supportsStats && len(result.Players) > 0 {
			continue
		}
		var ack protocol.MatchResult
		if access.Read(filepath.Join(dir, "result-ack-"+id+".json"), &ack) == nil && ack.Equal(result) {
			continue
		}
		return &result
	}
	return nil
}

func (m *Manager) AcknowledgeMatchResult(name string, ack protocol.MatchResult) error {
	if _, err := m.Dir(name); err != nil || !ack.Valid() {
		return errors.New("invalid match result acknowledgement")
	}
	dir := m.accessDir(name)
	result, err := readMatchResult(filepath.Join(dir, "result-finished-"+ack.SetupID+".json"))
	if err != nil || !result.Equal(ack) {
		return errors.New("match result acknowledgement does not match the local record")
	}
	return access.Write(filepath.Join(dir, "result-ack-"+ack.SetupID+".json"), ack)
}
