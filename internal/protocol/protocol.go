package protocol

import (
	"math"
	"slices"
	"strings"
	"time"
)

var Maps = []string{"de_mirage", "de_inferno", "de_dust2", "de_nuke", "de_ancient", "de_anubis", "de_train", "de_vertigo"}

func ValidMap(name string) bool {
	for _, m := range Maps {
		if m == name {
			return true
		}
	}
	return false
}

func ValidCommandID(id string) bool {
	if len(id) != 26 {
		return false
	}
	for _, r := range id {
		if !((r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7')) {
			return false
		}
	}
	return true
}

func ValidReporterHash(hash string) bool {
	return len(hash) == 64 && strings.Trim(hash, "0123456789abcdef") == ""
}

type Host struct {
	AvailableMaps           []string          `json:"availableMaps"`
	WorkshopMaps            []string          `json:"-"`
	WorkshopMapsSeen        *time.Time        `json:"-"`
	RestartPhase            string            `json:"restartPhase"`
	CanRestart              bool              `json:"restartSupported"`
	Preset                  *string           `json:"preset"`
	AppliedPreset           *string           `json:"-"`
	ActivePreset            string            `json:"activePreset"`
	PresetApplied           bool              `json:"presetApplied"`
	PresetStatus            string            `json:"presetStatus"`
	ModeCommandMode         string            `json:"modeCommandMode"`
	ModeCommandStatus       string            `json:"modeCommandStatus"`
	ModeCommandError        string            `json:"modeCommandError"`
	ModeCommandID           string            `json:"-"`
	ModeCommandRequestedAt  *time.Time        `json:"-"`
	CanPreset               bool              `json:"-"`
	CanLiveMode             bool              `json:"-"`
	CanLoadMatch            bool              `json:"canLoadMatch"`
	SupportsManualReady     bool              `json:"supportsManualReady"`
	SupportsMatchSeries     bool              `json:"supportsMatchSeries"`
	CanObserveMap           bool              `json:"canObserveMap"`
	CanReporterUpdate       bool              `json:"-"`
	CanUpdateGame           bool              `json:"gameUpdateSupported"`
	AllowPlayerRooms        bool              `json:"allowPlayerRooms"`
	ReporterSourceHash      string            `json:"-"`
	ReporterInstalledHash   string            `json:"-"`
	ReporterCommandID       string            `json:"-"`
	ReporterRequestedHash   string            `json:"-"`
	ReporterUpdateStatus    string            `json:"reporterUpdateStatus"`
	ReporterUpdateError     string            `json:"reporterUpdateError"`
	GameUpdateIssued        bool              `json:"-"`
	GameUpdateStatus        string            `json:"gameUpdateStatus"`
	GameUpdateError         string            `json:"gameUpdateError"`
	GameBuildStatus         string            `json:"gameBuildStatus"`
	GameBuildInstalled      string            `json:"gameBuildInstalled"`
	GameBuildLatest         string            `json:"gameBuildLatest"`
	GameBuildCheckedAt      *time.Time        `json:"gameBuildCheckedAt"`
	ReporterUpdateAvailable bool              `json:"reporterUpdateAvailable"`
	ReporterInventory       ReporterInventory `json:"reporterInventory"`
	ReporterRequestedAt     *time.Time        `json:"-"`
	MatchPhase              string            `json:"matchPhase"`
	MatchPaused             bool              `json:"matchPaused"`
	MatchControlAction      string            `json:"matchControlAction"`
	MatchControlStatus      string            `json:"matchControlStatus"`
	MatchControlError       string            `json:"matchControlError"`
	MatchControlID          string            `json:"-"`
	MatchControlRequestedAt *time.Time        `json:"-"`
	CanPauseMatch           bool              `json:"-"`
	CanResumeMatch          bool              `json:"-"`
	ID                      string            `json:"id"`
	MachineID               string            `json:"machineId"`
	Name                    string            `json:"name"`
	Address                 string            `json:"address"`
	Port                    int               `json:"port"`
	Map                     string            `json:"map"`
	DesiredState            string            `json:"desiredState"`
	ObservedState           string            `json:"observedState"`
	ActiveMap               string            `json:"activeMap"`
	Driver                  string            `json:"driver"`
	Error                   string            `json:"error"`
	Players                 *int              `json:"players"`
	MaxPlayers              *int              `json:"maxPlayers"`
	LogTail                 string            `json:"logTail"`
	AccessStatus            string            `json:"accessStatus"`
	LastSeen                *time.Time        `json:"lastSeen"`
	Online                  bool              `json:"online"`
	Revision                int64             `json:"revision"`
	Status                  string            `json:"status"`
	Actions                 Actions           `json:"actions"`
	Removing                bool              `json:"removing"`
	CanRemove               bool              `json:"-"`
	Whitelist               []string          `json:"whitelist"`
	WhitelistHash           string            `json:"-"`
	WhitelistApplied        bool              `json:"whitelistApplied"`
	CanWhitelist            bool              `json:"-"`
}

// ReporterInventory describes files last observed by the agent, not loaded code.
type ReporterInventory struct {
	Status        string     `json:"status"`
	InstalledHash string     `json:"installedHash"`
	StagedHash    string     `json:"stagedHash"`
	CheckedAt     *time.Time `json:"checkedAt"`
}

type Actions struct {
	Restart           bool `json:"restart"`
	CancelRestart     bool `json:"cancelRestart"`
	OpenMatch         bool `json:"openMatch"`
	UpdateReporter    bool `json:"updateReporter"`
	UpdateGame        bool `json:"updateGame"`
	StopAndUpdate     bool `json:"stopAndUpdate"`
	CancelGameUpdate  bool `json:"cancelGameUpdate"`
	ChangePreset      bool `json:"changePreset"`
	SwitchMode        bool `json:"switchMode"`
	PauseMatch        bool `json:"pauseMatch"`
	ResumeMatch       bool `json:"resumeMatch"`
	Start             bool `json:"start"`
	Stop              bool `json:"stop"`
	ChangeMap         bool `json:"changeMap"`
	Remove            bool `json:"remove"`
	ChangeWhitelist   bool `json:"changeWhitelist"`
	ChangePlayerRooms bool `json:"changePlayerRooms"`
}

type Workspace struct {
	Players     []Player     `json:"players"`
	Teams       []Team       `json:"teams"`
	Tournaments []Tournament `json:"tournaments"`
	Presets     []string     `json:"presets"`
	CapturedAt  string       `json:"capturedAt"`
	Hosts       []Host       `json:"hosts"`
	Matches     []Match      `json:"matches"`
	Maps        []string     `json:"maps"`
	Machines    []Machine    `json:"machines"`
}

type TeamActions struct {
	Invite          bool `json:"invite"`
	Leave           bool `json:"leave"`
	RemoveMember    bool `json:"removeMember"`
	TransferCaptain bool `json:"transferCaptain"`
	Disband         bool `json:"disband"`
}

type Team struct {
	Revision       int64        `json:"revision"`
	RosterLocked   bool         `json:"rosterLocked"`
	Registered     bool         `json:"registered"`
	Actions        TeamActions  `json:"actions"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	CaptainSteamID string       `json:"captainSteamId"`
	Members        []TeamMember `json:"members"`
	InviteCode     string       `json:"inviteCode,omitempty"`
}

type TeamMember struct {
	SteamID  string `json:"steamId"`
	GameName string `json:"gameName"`
}

type TournamentStanding struct {
	TeamID   string `json:"teamId"`
	TeamName string `json:"teamName"`
	Played   int    `json:"played"`
	Wins     int    `json:"wins"`
	Losses   int    `json:"losses"`
	Rank     int    `json:"rank"`
}

type Tournament struct {
	CanCancel       bool                 `json:"canCancel"`
	Format          string               `json:"format"`
	Standings       []TournamentStanding `json:"standings"`
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	PlayersPerSide  int                  `json:"playersPerSide"`
	MapPool         []string             `json:"mapPool"`
	BestOf          int                  `json:"bestOf"`
	Status          string               `json:"status"`
	WinnerTeamID    string               `json:"winnerTeamId"`
	Entries         []TournamentEntry    `json:"entries"`
	Matches         []TournamentMatch    `json:"matches"`
	CanCheckIn      bool                 `json:"canCheckIn"`
	CanBuildBracket bool                 `json:"canBuildBracket"`
	MyTeamRevision  int64                `json:"myTeamRevision"`
	MyTeamID        string               `json:"myTeamId,omitempty"`
	MyEntryStatus   string               `json:"myEntryStatus,omitempty"`
	CanRegisterTeam bool                 `json:"canRegisterTeam"`
	CanCheckInTeam  bool                 `json:"canCheckInTeam"`
	CanWithdrawTeam bool                 `json:"canWithdrawTeam"`
}

type TournamentEntry struct {
	TeamID   string `json:"teamId"`
	TeamName string `json:"teamName"`
	Status   string `json:"status"`
	Members  int    `json:"members,omitempty"`
}

type TournamentDispute struct {
	Reason     string `json:"reason"`
	Resolution string `json:"resolution,omitempty"`
	Revision   int64  `json:"revision"`
}

type TournamentMatch struct {
	WinnerHistory       []TournamentWinnerChange `json:"winnerHistory,omitempty"`
	ResultRevision      int64                    `json:"resultRevision"`
	CanCorrectWinner    bool                     `json:"canCorrectWinner"`
	WinnerCorrected     bool                     `json:"winnerCorrected"`
	BracketStage        string                   `json:"bracketStage"`
	Dispute             *TournamentDispute       `json:"dispute,omitempty"`
	CanDispute          bool                     `json:"canDispute,omitempty"`
	DisputeParticipants []string                 `json:"-"`
	DisputeEligible     bool                     `json:"-"`
	ScheduledAt         *time.Time               `json:"scheduledAt"`
	ScheduleRevision    int64                    `json:"scheduleRevision"`
	ReportedCT          *int                     `json:"reportedCT"`
	ReportedT           *int                     `json:"reportedT"`
	TeamAScore          *int                     `json:"teamAScore"`
	TeamBScore          *int                     `json:"teamBScore"`
	ResultCT            *int                     `json:"resultCT"`
	ResultT             *int                     `json:"resultT"`
	ID                  string                   `json:"id"`
	Round               int                      `json:"round"`
	Number              int                      `json:"number"`
	Status              string                   `json:"status"`
	TeamA               *TournamentEntry         `json:"teamA"`
	TeamB               *TournamentEntry         `json:"teamB"`
	WinnerTeamID        string                   `json:"winnerTeamId"`
	RoomID              string                   `json:"roomId,omitempty"`
	RoomStatus          string                   `json:"roomStatus,omitempty"`
	Map                 string                   `json:"map,omitempty"`
	VetoBans            []string                 `json:"vetoBans"`
	VetoMaps            []string                 `json:"vetoMaps"`
	MapSequence         []string                 `json:"mapSequence"`
	VetoStep            int                      `json:"vetoStep"`
	VetoTurnTeamID      string                   `json:"vetoTurnTeamId,omitempty"`
	VetoTurnTeam        string                   `json:"vetoTurnTeam,omitempty"`
	TeamBSide           string                   `json:"teamBSide,omitempty"`
	CanVeto             bool                     `json:"canVeto"`
	CanReportWinner     bool                     `json:"canReportWinner"`
	CanOpenRoom         bool                     `json:"canOpenRoom"`
	CanSchedule         bool                     `json:"canSchedule"`
}

type TournamentWinnerChange struct {
	PreviousWinner string    `json:"previousWinner"`
	NewWinner      string    `json:"newWinner"`
	Revision       int64     `json:"revision"`
	ChangedAt      time.Time `json:"changedAt"`
}

type Player struct {
	SteamID  string `json:"steamId"`
	GameName string `json:"gameName"`
}

type LiveScore struct {
	SetupID    string `json:"setupId"`
	Map        string `json:"map"`
	CT         int    `json:"ct"`
	T          int    `json:"t"`
	ObservedAt int64  `json:"observedAt"`
}

func (s *LiveScore) Valid() bool {
	return s != nil && ValidCommandID(s.SetupID) && ValidMap(s.Map) && s.CT >= 0 && s.CT <= 99 && s.T >= 0 && s.T <= 99 && s.ObservedAt > 0
}

type Match struct {
	CanConfigurePauseVoting bool               `json:"canConfigurePauseVoting"`
	PauseVoteThreshold      int                `json:"pauseVoteThreshold"`
	PauseVotes              int                `json:"pauseVotes"`
	ControlRevision         int64              `json:"controlRevision"`
	MapVotes                []MapVote          `json:"mapVotes"`
	LiveScore               *LiveScore         `json:"liveScore"`
	ID                      string             `json:"id"`
	HostID                  string             `json:"hostId"`
	HostName                string             `json:"hostName"`
	Title                   string             `json:"title"`
	Map                     string             `json:"map"`
	Overtime                bool               `json:"overtime"`
	ManualReady             bool               `json:"manualReady"`
	Status                  string             `json:"status"`
	Players                 int                `json:"players"`
	PlayersPerSide          int                `json:"playersPerSide"`
	CTPlayers               int                `json:"ctPlayers"`
	TPlayers                int                `json:"tPlayers"`
	Ready                   int                `json:"ready"`
	Participants            []MatchParticipant `json:"participants"`
	LoadStatus              string             `json:"loadStatus"`
	LoadError               string             `json:"loadError"`
	CanLoad                 bool               `json:"canLoad"`
	LoadBlockers            []string           `json:"loadBlockers"`
	LoadBlockedByFailure    bool               `json:"loadBlockedByFailure"`
	CanCancelLoad           bool               `json:"canCancelLoad"`
	CancelStatus            string             `json:"cancelStatus"`
	CancelError             string             `json:"cancelError"`
	CanClose                bool               `json:"canClose"`
	CanReopen               bool               `json:"canReopen"`
	CanEnd                  bool               `json:"canEnd"`
	CanEdit                 bool               `json:"canEdit"`
	CanRemovePlayer         bool               `json:"canRemovePlayer"`
	CanShuffle              bool               `json:"canShuffle"`
	SettingsRevision        int64              `json:"settingsRevision"`
	ResultCT                *int               `json:"resultCT"`
	ResultT                 *int               `json:"resultT"`
	ResultRevision          int64              `json:"resultRevision"`
	CanEditResult           bool               `json:"canEditResult"`
	SeriesMatch             bool               `json:"seriesMatch"`
	SetupUnloaded           bool               `json:"setupUnloaded"`
	ReportedCT              *int               `json:"reportedCT"`
	ReportedT               *int               `json:"reportedT"`
	EndedAt                 *time.Time         `json:"endedAt"`
	CreatedAt               time.Time          `json:"createdAt"`
}

type MapVote struct {
	Map   string `json:"map"`
	Votes int    `json:"votes"`
}

type MatchParticipant struct {
	RemovalToken  string      `json:"removalToken,omitempty"`
	ReportedStats *MatchStats `json:"reportedStats"`
	Stats         *MatchStats `json:"stats"`
	SteamID       string      `json:"steamId,omitempty"`
	GameName      string      `json:"gameName"`
	Side          string      `json:"side"`
	Ready         bool        `json:"ready"`
}

type MatchStats struct {
	Kills   int `json:"kills"`
	Deaths  int `json:"deaths"`
	Assists int `json:"assists"`
}

type MatchSetup struct {
	ID             string             `json:"id"`
	RequestedAt    int64              `json:"requestedAt,omitempty"`
	FenceRequired  bool               `json:"fenceRequired,omitempty"`
	ManualReady    bool               `json:"manualReady,omitempty"`
	Map            string             `json:"map"`
	Maps           []string           `json:"maps,omitempty"`
	MapSides       []string           `json:"mapSides,omitempty"`
	Overtime       bool               `json:"overtime"`
	PlayersPerSide int                `json:"playersPerSide"`
	CT             []MatchSetupPlayer `json:"ct"`
	T              []MatchSetupPlayer `json:"t"`
}

type MatchSetupPlayer struct {
	SteamID  string `json:"steamId"`
	GameName string `json:"gameName"`
}

const MaxServers = 64

type Machine struct {
	Resources    *MachineResources `json:"resources"`
	Storage      *MachineStorage   `json:"storage"`
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Driver       string            `json:"driver"`
	LastSeen     *time.Time        `json:"lastSeen"`
	Online       bool              `json:"online"`
	CreateServer bool              `json:"createServer"`
}

type MachineResources struct {
	CPUPercent           *float64 `json:"cpuPercent"`
	MemoryTotalBytes     int64    `json:"memoryTotalBytes"`
	MemoryAvailableBytes int64    `json:"memoryAvailableBytes"`
}

func (s *MachineResources) Valid() bool {
	return s == nil || (s.MemoryTotalBytes > 0 && s.MemoryAvailableBytes >= 0 && s.MemoryAvailableBytes <= s.MemoryTotalBytes && (s.CPUPercent == nil || (!math.IsNaN(*s.CPUPercent) && !math.IsInf(*s.CPUPercent, 0) && *s.CPUPercent >= 0 && *s.CPUPercent <= 100)))
}

type MachineStorage struct {
	TotalBytes     int64 `json:"totalBytes"`
	AvailableBytes int64 `json:"availableBytes"`
}

func (s *MachineStorage) Valid() bool {
	return s == nil || (s.TotalBytes > 0 && s.AvailableBytes >= 0 && s.AvailableBytes <= s.TotalBytes)
}

type ServerReport struct {
	ID string `json:"id"`
	Report
}

type ServerDesired struct {
	ResultAck *MatchResult `json:"resultAck,omitempty"`
	ID        string       `json:"id"`
	Remove    bool         `json:"remove,omitempty"`
	Desired
}

type MatchResult struct {
	SetupID    string           `json:"setupId"`
	CT         int              `json:"ct"`
	T          int              `json:"t"`
	Team1Score *int             `json:"team1Score,omitempty"`
	Team2Score *int             `json:"team2Score,omitempty"`
	Players    []ReportedPlayer `json:"players,omitempty"`
}

type ReportedPlayer struct {
	SteamID string `json:"steamId"`
	Kills   int    `json:"kills"`
	Deaths  int    `json:"deaths"`
	Assists int    `json:"assists"`
}

func (r *MatchResult) Valid() bool {
	if r == nil || !ValidCommandID(r.SetupID) || r.CT < 0 || r.CT > 99 || r.T < 0 || r.T > 99 || len(r.Players) > 10 {
		return false
	}
	if (r.Team1Score == nil) != (r.Team2Score == nil) || (r.Team1Score != nil && (*r.Team1Score < 0 || *r.Team1Score > 5 || *r.Team2Score < 0 || *r.Team2Score > 5)) {
		return false
	}
	for i, p := range r.Players {
		if ValidateWhitelist([]string{p.SteamID}) != nil || (i > 0 && r.Players[i-1].SteamID >= p.SteamID) || p.Kills < 0 || p.Kills > 999 || p.Deaths < 0 || p.Deaths > 999 || p.Assists < 0 || p.Assists > 999 {
			return false
		}
	}
	return true
}

func (r MatchResult) Equal(other MatchResult) bool {
	return r.SetupID == other.SetupID && r.CT == other.CT && r.T == other.T && sameInt(r.Team1Score, other.Team1Score) && sameInt(r.Team2Score, other.Team2Score) && slices.Equal(r.Players, other.Players)
}

func sameInt(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

type MachineReport struct {
	SupportsRestart          bool              `json:"supportsRestart,omitempty"`
	Resources                *MachineResources `json:"resources,omitempty"`
	Storage                  *MachineStorage   `json:"storage,omitempty"`
	SupportsPresets          bool              `json:"supportsPresets,omitempty"`
	SupportsLiveMode         bool              `json:"supportsLiveMode,omitempty"`
	SupportsMatchSetup       bool              `json:"supportsMatchSetup,omitempty"`
	SupportsMatchSetupExpiry bool              `json:"supportsMatchSetupExpiry,omitempty"`
	SupportsMatchCancel      bool              `json:"supportsMatchCancel,omitempty"`
	SupportsReporterUpdate   bool              `json:"supportsReporterUpdate,omitempty"`
	SupportsGameUpdate       bool              `json:"supportsGameUpdate,omitempty"`
	SupportsMatchControl     bool              `json:"supportsMatchControl,omitempty"`
	SupportsMapObservation   bool              `json:"supportsMapObservation,omitempty"`
	Driver                   string            `json:"driver"`
	SupportsWhitelist        bool              `json:"supportsWhitelist,omitempty"`
	SupportsRemoval          bool              `json:"supportsRemoval,omitempty"`
	Servers                  []ServerReport    `json:"servers"`
}

type MachineDesired struct {
	SupportsResultStats bool            `json:"supportsResultStats,omitempty"`
	Servers             []ServerDesired `json:"servers"`
}

// Project is the single lifecycle/action policy used by reads and mutations.
func (h *Host) Project(now time.Time) {
	h.Online = h.LastSeen != nil && now.Sub(*h.LastSeen) < 20*time.Second
	h.AvailableMaps = append([]string{}, Maps...)
	if h.Online && h.Driver == "managed" && h.WorkshopMapsSeen != nil && !h.WorkshopMapsSeen.After(now) && now.Sub(*h.WorkshopMapsSeen) < 20*time.Second && ValidWorkshopMaps(h.WorkshopMaps) {
		h.AvailableMaps = append(h.AvailableMaps, h.WorkshopMaps...)
	}
	h.ReporterInventory = ReporterInventory{Status: "unknown"}
	if h.CanReporterUpdate && h.Driver == "managed" {
		h.ReporterInventory.InstalledHash = h.ReporterInstalledHash
		h.ReporterInventory.StagedHash = h.ReporterSourceHash
		h.ReporterInventory.CheckedAt = h.LastSeen
		if h.Online && ValidReporterHash(h.ReporterSourceHash) {
			switch {
			case h.ReporterInstalledHash == "":
				h.ReporterInventory.Status = "unavailable"
			case ValidReporterHash(h.ReporterInstalledHash) && h.ReporterInstalledHash == h.ReporterSourceHash:
				h.ReporterInventory.Status = "current"
			case ValidReporterHash(h.ReporterInstalledHash):
				h.ReporterInventory.Status = "different"
			}
		}
	}
	h.SupportsManualReady = h.SupportsManualReady && h.Online && h.Driver == "managed"
	h.GameBuildStatus = "unknown"
	if h.Online && h.GameBuildCheckedAt != nil && !h.GameBuildCheckedAt.After(now.Add(5*time.Minute)) && now.Sub(*h.GameBuildCheckedAt) < 2*time.Hour && h.GameBuildInstalled != "" && h.GameBuildLatest != "" && h.ObservedState != "installing" && h.GameUpdateStatus != "pending" {
		if h.GameBuildInstalled == h.GameBuildLatest {
			h.GameBuildStatus = "current"
		} else {
			h.GameBuildStatus = "outdated"
		}
	}
	h.CanObserveMap = h.CanObserveMap && h.Online && h.Driver == "managed"
	h.Actions = Actions{}
	h.PresetApplied = h.Preset == nil || (h.Online && h.CanPreset && h.AppliedPreset != nil && *h.Preset == *h.AppliedPreset)
	h.PresetStatus = ""
	if h.Preset != nil && *h.Preset != "" {
		switch {
		case !h.Online:
			h.PresetStatus = "unknown"
		case h.ObservedState != "running":
			h.PresetStatus = "pending"
		case h.ActivePreset == *h.Preset:
			h.PresetStatus = "verified"
		case h.ActivePreset != "":
			h.PresetStatus = "mismatch"
		default:
			h.PresetStatus = "unknown"
		}
	}
	if !h.Online || (h.ObservedState != "running" && h.ObservedState != "starting") {
		h.ActivePreset = ""
	}
	if !h.Online || h.ObservedState != "running" || h.DesiredState != "running" || h.AccessStatus != "verified" || h.ActivePreset != "match" {
		h.MatchPhase = "unknown"
		h.MatchPaused = false
		h.CanPauseMatch = false
		h.CanResumeMatch = false
	}
	h.WhitelistApplied = h.Whitelist == nil || (h.Online && h.CanWhitelist && h.WhitelistHash == WhitelistHash(h.Whitelist))
	h.ReporterUpdateAvailable = h.Online && h.CanReporterUpdate && ValidReporterHash(h.ReporterSourceHash) && (h.ReporterInstalledHash == "" || ValidReporterHash(h.ReporterInstalledHash)) && h.ReporterSourceHash != h.ReporterInstalledHash
	if h.Removing {
		h.Status = "removing"
		if !h.Online && h.AccessStatus != "" {
			h.AccessStatus = "unknown"
		}
		return
	}
	h.Actions.CancelGameUpdate = h.MachineID != "" && h.GameUpdateStatus == "pending" && !h.GameUpdateIssued
	if !h.Online {
		if h.AccessStatus != "" {
			h.AccessStatus = "unknown"
		}
		h.Status = "offline"
		return
	}
	h.Actions.ChangePlayerRooms = h.MachineID != "" && h.Driver == "managed"
	h.Actions.Stop = h.DesiredState == "running"
	h.Actions.Start = h.DesiredState == "stopped" && (h.ObservedState == "stopped" || h.ObservedState == "error" || h.ObservedState == "uninstalled")
	h.Actions.ChangeMap = h.DesiredState == "stopped" && (h.ObservedState == "stopped" || h.ObservedState == "uninstalled")
	h.Actions.ChangeWhitelist = h.CanWhitelist && h.MachineID != "" && h.DesiredState == "stopped" && h.ObservedState == "stopped"
	h.Actions.ChangePreset = h.CanPreset && h.MachineID != "" && h.Actions.ChangeMap
	h.Actions.SwitchMode = h.CanLiveMode && h.DesiredState == "running" && h.ObservedState == "running" && (h.ActivePreset == "match" || h.ActivePreset == "practice") && h.ModeCommandStatus != "pending"
	canControlMatch := h.MachineID != "" && h.Driver == "managed" && h.Error == "" && h.DesiredState == "running" && h.ObservedState == "running" && h.ModeCommandStatus != "pending" && h.MatchControlStatus != "pending"
	h.Actions.PauseMatch = canControlMatch && h.CanPauseMatch && h.MatchPhase == "live"
	h.Actions.ResumeMatch = canControlMatch && h.CanResumeMatch && h.MatchPhase == "paused"
	h.Actions.Start = h.Actions.Start && h.WhitelistApplied && h.PresetApplied && slices.Contains(h.AvailableMaps, h.Map)
	h.Actions.Remove = h.CanRemove && h.MachineID != "" && h.Actions.ChangeMap
	h.Actions.UpdateReporter = h.ReporterUpdateAvailable && h.MachineID != "" && h.Driver == "managed" && h.DesiredState == "stopped" && h.ObservedState == "stopped" && h.ReporterUpdateStatus != "pending"
	h.Actions.UpdateGame = h.CanUpdateGame && h.Online && h.MachineID != "" && h.Driver == "managed" && h.DesiredState == "stopped" && h.ObservedState == "stopped" && h.GameUpdateStatus != "pending"
	h.Actions.StopAndUpdate = h.CanUpdateGame && h.MachineID != "" && h.Driver == "managed" && h.DesiredState == "running" && h.ObservedState == "running" && h.Error == "" && h.GameUpdateStatus != "pending" && h.ReporterUpdateStatus != "pending" && h.ModeCommandStatus != "pending" && h.MatchControlStatus != "pending" && h.MatchPhase != "live" && h.MatchPhase != "paused" && h.MatchPhase != "setup" && (h.ActivePreset != "match" || h.MatchPhase == "idle")
	if h.GameUpdateStatus == "pending" || h.ReporterUpdateStatus == "pending" {
		h.Actions.Start = false
		h.Actions.ChangeMap = false
		h.Actions.ChangeWhitelist = false
		h.Actions.ChangePreset = false
		h.Actions.Remove = false
		h.Actions.UpdateGame = false
		h.Actions.UpdateReporter = false
		h.Actions.StopAndUpdate = false
		h.Actions.ChangePlayerRooms = false
	}
	restarting := h.RestartPhase == "stopping" || h.RestartPhase == "starting" || h.RestartPhase == "cancelling"
	h.Actions.Restart = h.CanRestart && h.MachineID != "" && h.Driver == "managed" && h.DesiredState == "running" && h.ObservedState == "running" && h.Error == "" && h.WhitelistApplied && h.PresetApplied && h.GameUpdateStatus != "pending" && h.ReporterUpdateStatus != "pending" && h.ModeCommandStatus != "pending" && h.MatchControlStatus != "pending" && h.MatchPhase != "live" && h.MatchPhase != "paused" && h.MatchPhase != "setup" && (h.ActivePreset != "match" || h.MatchPhase == "idle") && !restarting && slices.Contains(h.AvailableMaps, h.Map)
	if restarting {
		h.Actions.ChangePlayerRooms = false
		h.Actions.StopAndUpdate = false
		h.Actions.Restart, h.Actions.Start, h.Actions.ChangeMap, h.Actions.ChangeWhitelist, h.Actions.ChangePreset, h.Actions.Remove, h.Actions.UpdateReporter, h.Actions.UpdateGame, h.Actions.SwitchMode = false, false, false, false, false, false, false, false, false
		h.Actions.PauseMatch, h.Actions.ResumeMatch = false, false
		h.Actions.CancelRestart = h.RestartPhase != "cancelling"
	}
	switch {
	case h.Error != "" || h.ObservedState == "error":
		h.Status = "error"
	case h.ObservedState == "installing" && h.DesiredState == "running":
		h.Status = "installing"
	case h.ObservedState == "uninstalled" && h.DesiredState == "stopped":
		h.Status = "uninstalled"
	case h.DesiredState == "running" && h.ObservedState != "running":
		h.Status = "starting"
	case h.DesiredState == "stopped" && h.ObservedState != "stopped":
		h.Status = "stopping"
	default:
		h.Status = h.ObservedState
	}
}

type Desired struct {
	Restart        *RestartCommand `json:"restart,omitempty"`
	GameUpdate     *GameUpdate     `json:"gameUpdate,omitempty"`
	ReporterUpdate *ReporterUpdate `json:"reporterUpdate,omitempty"`
	Preset         *string         `json:"preset,omitempty"`
	LiveMode       *ModeCommand    `json:"liveMode,omitempty"`
	MatchSetup     *MatchSetup     `json:"matchSetup,omitempty"`
	MatchControl   *MatchControl   `json:"matchControl,omitempty"`
	MatchCancel    *MatchCancel    `json:"matchCancel,omitempty"`
	Whitelist      *[]string       `json:"whitelist,omitempty"`
	State          string          `json:"state"`
	Map            string          `json:"map"`
	Port           int             `json:"port"`
}

type GameUpdate struct {
	ID string `json:"id"`
}

// RestartCommand is a fixed, request-fenced lifecycle instruction.
type RestartCommand struct {
	ID    string `json:"id"`
	Phase string `json:"phase"`
}

func (c *RestartCommand) Valid() bool {
	return c != nil && ValidCommandID(c.ID) && (c.Phase == "stop" || c.Phase == "start" || c.Phase == "cancel")
}

type ReporterUpdate struct {
	ID   string `json:"id"`
	Hash string `json:"hash"`
}

type ModeCommand struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	RequestedAt int64  `json:"requestedAt,omitempty"`
}

type MatchControl struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	SetupID string `json:"setupId,omitempty"`
}

type MatchCancel struct {
	ID      string `json:"id"`
	SetupID string `json:"setupId"`
}

type Report struct {
	WorkshopMaps               *[]string    `json:"workshopMaps,omitempty"`
	RestartID                  string       `json:"restartId,omitempty"`
	RestartPhase               string       `json:"restartPhase,omitempty"`
	LiveScore                  *LiveScore   `json:"liveScore,omitempty"`
	JoinPassword               string       `json:"joinPassword,omitempty"`
	GameUpdateID               string       `json:"gameUpdateId,omitempty"`
	GameUpdateStatus           string       `json:"gameUpdateStatus,omitempty"`
	GameUpdateError            string       `json:"gameUpdateError,omitempty"`
	GameBuildInstalled         string       `json:"gameBuildInstalled,omitempty"`
	GameBuildLatest            string       `json:"gameBuildLatest,omitempty"`
	GameBuildCheckedAt         int64        `json:"gameBuildCheckedAt,omitempty"`
	MatchResult                *MatchResult `json:"matchResult,omitempty"`
	ReporterSourceHash         string       `json:"reporterSourceHash,omitempty"`
	ReporterInstalledHash      string       `json:"reporterInstalledHash,omitempty"`
	ReporterUpdateID           string       `json:"reporterUpdateId,omitempty"`
	ReporterUpdateStatus       string       `json:"reporterUpdateStatus,omitempty"`
	ReporterUpdateError        string       `json:"reporterUpdateError,omitempty"`
	AppliedPreset              *string      `json:"appliedPreset,omitempty"`
	ActivePreset               string       `json:"activePreset,omitempty"`
	CanLiveMode                bool         `json:"canLiveMode,omitempty"`
	CanLoadMatch               bool         `json:"canLoadMatch,omitempty"`
	SupportsManualReady        bool         `json:"supportsManualReady,omitempty"`
	SupportsMatchSeries        bool         `json:"supportsMatchSeries,omitempty"`
	SupportsFencedMatchControl bool         `json:"supportsFencedMatchControl,omitempty"`
	ModeCommandID              string       `json:"modeCommandId,omitempty"`
	ModeCommandStatus          string       `json:"modeCommandStatus,omitempty"`
	ModeCommandError           string       `json:"modeCommandError,omitempty"`
	MatchSetupID               string       `json:"matchSetupId,omitempty"`
	MatchSetupStatus           string       `json:"matchSetupStatus,omitempty"`
	MatchSetupError            string       `json:"matchSetupError,omitempty"`
	MatchPhase                 string       `json:"matchPhase,omitempty"`
	ActiveMatchSetupID         string       `json:"activeMatchSetupId,omitempty"`
	SupportsMatchFence         bool         `json:"supportsMatchFence,omitempty"`
	MatchCancelID              string       `json:"matchCancelId,omitempty"`
	MatchCancelSetupID         string       `json:"matchCancelSetupId,omitempty"`
	MatchCancelStatus          string       `json:"matchCancelStatus,omitempty"`
	MatchCancelError           string       `json:"matchCancelError,omitempty"`
	CanPauseMatch              bool         `json:"canPauseMatch,omitempty"`
	CanResumeMatch             bool         `json:"canResumeMatch,omitempty"`
	MatchControlID             string       `json:"matchControlId,omitempty"`
	MatchControlAction         string       `json:"matchControlAction,omitempty"`
	MatchControlStatus         string       `json:"matchControlStatus,omitempty"`
	MatchControlError          string       `json:"matchControlError,omitempty"`
	WhitelistHash              string       `json:"whitelistHash,omitempty"`
	State                      string       `json:"state"`
	ActiveMap                  string       `json:"activeMap"`
	Driver                     string       `json:"driver"`
	Error                      string       `json:"error"`
	Players                    *int         `json:"players,omitempty"`
	MaxPlayers                 *int         `json:"maxPlayers,omitempty"`
	LogTail                    string       `json:"logTail,omitempty"`
	AccessStatus               string       `json:"accessStatus,omitempty"`
}
