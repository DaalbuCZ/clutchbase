package protocol

import "time"

var Maps = []string{"de_mirage", "de_inferno", "de_dust2", "de_nuke", "de_ancient", "de_anubis", "de_train", "de_vertigo"}

func ValidMap(name string) bool {
	for _, m := range Maps {
		if m == name {
			return true
		}
	}
	return false
}

type Host struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Port          int        `json:"port"`
	Map           string     `json:"map"`
	DesiredState  string     `json:"desiredState"`
	ObservedState string     `json:"observedState"`
	ActiveMap     string     `json:"activeMap"`
	Driver        string     `json:"driver"`
	Error         string     `json:"error"`
	LastSeen      *time.Time `json:"lastSeen"`
	Online        bool       `json:"online"`
	Revision      int64      `json:"revision"`
	Status        string     `json:"status"`
	Actions       Actions    `json:"actions"`
}

type Actions struct {
	Start     bool `json:"start"`
	Stop      bool `json:"stop"`
	ChangeMap bool `json:"changeMap"`
}

type Workspace struct {
	CapturedAt string   `json:"capturedAt"`
	Hosts      []Host   `json:"hosts"`
	Maps       []string `json:"maps"`
}

// Project is the single lifecycle/action policy used by reads and mutations.
func (h *Host) Project(now time.Time) {
	h.Online = h.LastSeen != nil && now.Sub(*h.LastSeen) < 20*time.Second
	h.Actions = Actions{}
	if !h.Online {
		h.Status = "offline"
		return
	}
	h.Actions.Stop = h.DesiredState == "running"
	h.Actions.Start = h.DesiredState == "stopped" && (h.ObservedState == "stopped" || h.ObservedState == "error")
	h.Actions.ChangeMap = h.DesiredState == "stopped" && h.ObservedState == "stopped"
	switch {
	case h.Error != "" || h.ObservedState == "error":
		h.Status = "error"
	case h.DesiredState == "running" && h.ObservedState != "running":
		h.Status = "starting"
	case h.DesiredState == "stopped" && h.ObservedState != "stopped":
		h.Status = "stopping"
	default:
		h.Status = h.ObservedState
	}
}

type Desired struct {
	State string `json:"state"`
	Map   string `json:"map"`
}

type Report struct {
	State     string `json:"state"`
	ActiveMap string `json:"activeMap"`
	Driver    string `json:"driver"`
	Error     string `json:"error"`
}
