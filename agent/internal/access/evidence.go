// Package access exchanges private, per-launch evidence with the local game plugin.
package access

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"clutchbase/internal/protocol"
)

const Freshness = 5 * time.Second

type Request struct {
	Nonce        string `json:"nonce"`
	RosterHash   string `json:"rosterHash"`
	ProcessGroup int    `json:"processGroup"`
}

type Evidence struct {
	LiveScore                  *protocol.LiveScore `json:"liveScore,omitempty"`
	Mode                       string              `json:"mode,omitempty"`
	CanSwitchMode              bool                `json:"canSwitchMode,omitempty"`
	CanLoadMatch               bool                `json:"canLoadMatch,omitempty"`
	SupportsManualReady        bool                `json:"supportsManualReady,omitempty"`
	SupportsMatchSeries        bool                `json:"supportsMatchSeries,omitempty"`
	SupportsFencedMatchControl bool                `json:"supportsFencedMatchControl,omitempty"`
	MatchPhase                 string              `json:"matchPhase,omitempty"`
	ActiveMatchSetupID         string              `json:"activeMatchSetupId,omitempty"`
	CanPauseMatch              bool                `json:"canPauseMatch,omitempty"`
	CanResumeMatch             bool                `json:"canResumeMatch,omitempty"`
	SupportsMatchFence         bool                `json:"supportsMatchFence,omitempty"`
	Request
	ModeCommandID      string `json:"modeCommandId,omitempty"`
	ModeCommandStatus  string `json:"modeCommandStatus,omitempty"`
	ModeCommandError   string `json:"modeCommandError,omitempty"`
	MatchSetupID       string `json:"matchSetupId,omitempty"`
	MatchSetupStatus   string `json:"matchSetupStatus,omitempty"`
	MatchSetupError    string `json:"matchSetupError,omitempty"`
	MatchControlID     string `json:"matchControlId,omitempty"`
	MatchControlAction string `json:"matchControlAction,omitempty"`
	MatchControlStatus string `json:"matchControlStatus,omitempty"`
	MatchControlError  string `json:"matchControlError,omitempty"`
	MatchCancelID      string `json:"matchCancelId,omitempty"`
	MatchCancelSetupID string `json:"matchCancelSetupId,omitempty"`
	MatchCancelStatus  string `json:"matchCancelStatus,omitempty"`
	MatchCancelError   string `json:"matchCancelError,omitempty"`
	ProcessID          int    `json:"processId"`
	ReportedAt         int64  `json:"reportedAt"`
	Status             string `json:"status"`
}

type MatchSetupRequest struct {
	ID             string                      `json:"id"`
	FenceRequired  bool                        `json:"fenceRequired,omitempty"`
	ManualReady    bool                        `json:"manualReady,omitempty"`
	Map            string                      `json:"map"`
	Maps           []string                    `json:"maps,omitempty"`
	MapSides       []string                    `json:"mapSides,omitempty"`
	Overtime       bool                        `json:"overtime"`
	PlayersPerSide int                         `json:"playersPerSide"`
	CT             []protocol.MatchSetupPlayer `json:"ct"`
	T              []protocol.MatchSetupPlayer `json:"t"`
	RequestedAt    int64                       `json:"requestedAt"`
}

type ModeCommand struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	RequestedAt int64  `json:"requestedAt"`
}

type MatchControlRequest struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	SetupID     string `json:"setupId,omitempty"`
	RequestedAt int64  `json:"requestedAt"`
}

type MatchCancelRequest struct {
	ID      string `json:"id"`
	SetupID string `json:"setupId"`
}

func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func Read(path string, out any) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return errors.New("runtime whitelist evidence unavailable")
	}
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, out) != nil {
		return errors.New("runtime whitelist evidence invalid")
	}
	return nil
}

func Write(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".access-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func Verify(dir string, request Request, now time.Time) error {
	_, err := Verified(dir, request, now)
	return err
}

func Verified(dir string, request Request, now time.Time) (Evidence, error) {
	var evidence Evidence
	if err := Read(filepath.Join(dir, "evidence.json"), &evidence); err != nil {
		return Evidence{}, err
	}
	age := now.Sub(time.UnixMilli(evidence.ReportedAt))
	group, err := syscall.Getpgid(evidence.ProcessID)
	if err != nil || evidence.ProcessID <= 0 || request.Nonce == "" || request.ProcessGroup <= 0 || group != request.ProcessGroup || evidence.ProcessGroup != request.ProcessGroup || evidence.Nonce != request.Nonce || evidence.RosterHash != request.RosterHash || len(request.RosterHash) != 64 || age > Freshness || age < -time.Second || evidence.Status != "verified" {
		return Evidence{}, errors.New("runtime whitelist verification failed")
	}
	return evidence, nil
}
