package control

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var validBuildID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

func validReport(in protocol.Report) bool {
	restartOK := (in.RestartID == "" && in.RestartPhase == "") || (in.Driver == "managed" && protocol.ValidCommandID(in.RestartID) && (in.RestartPhase == "stopping" || in.RestartPhase == "stopped" || in.RestartPhase == "starting" || in.RestartPhase == "complete" || in.RestartPhase == "cancelled" || in.RestartPhase == "failed"))
	stateOK := in.State == "running" || in.State == "stopped" || in.State == "error" || in.State == "starting" || in.State == "stopping" || in.State == "uninstalled" || in.State == "installing" || in.State == "removed"
	driverOK := in.Driver == "simulated" || in.Driver == "systemd" || in.Driver == "managed"
	mapOK := in.ActiveMap == "" || protocol.ValidInstalledMap(in.ActiveMap)
	countOK := func(value *int) bool { return value == nil || (*value >= 0 && *value <= 255) }
	accessOK := in.AccessStatus == "" || (in.Driver == "managed" && (in.AccessStatus == "password" || in.AccessStatus == "pending" || in.AccessStatus == "blocked" || (in.AccessStatus == "verified" && (in.State == "running" || in.State == "starting"))))
	hashOK := in.WhitelistHash == "" || (len(in.WhitelistHash) == 64 && strings.Trim(in.WhitelistHash, "0123456789abcdef") == "")
	presetOK := (in.AppliedPreset == nil || protocol.ValidPreset(*in.AppliedPreset)) && protocol.ValidPreset(in.ActivePreset) && (in.ActivePreset == "" || (in.Driver == "managed" && in.AccessStatus == "verified" && (in.State == "running" || in.State == "starting")))
	modeOK := !in.CanLiveMode || (in.Driver == "managed" && in.AccessStatus == "verified" && (in.ActivePreset == "match" || in.ActivePreset == "practice"))
	matchCapabilityOK := !in.CanLoadMatch || (in.Driver == "managed" && in.AccessStatus == "verified" && in.ActivePreset == "match")
	matchSeriesCapabilityOK := !in.SupportsMatchSeries || (in.Driver == "managed" && in.AccessStatus == "verified" && in.ActivePreset == "match" && in.CanLoadMatch)
	matchPhaseOK := in.MatchPhase == "" || in.MatchPhase == "unknown" || ((in.MatchPhase == "idle" || in.MatchPhase == "setup" || in.MatchPhase == "live" || in.MatchPhase == "paused") && in.Driver == "managed" && in.AccessStatus == "verified" && in.ActivePreset == "match" && (in.State == "running" || in.State == "starting"))
	matchControlCapabilityOK := (!in.CanPauseMatch || in.MatchPhase == "live") && (!in.CanResumeMatch || in.MatchPhase == "paused") && (!in.CanPauseMatch && !in.CanResumeMatch || in.Driver == "managed" && in.AccessStatus == "verified" && in.ActivePreset == "match")
	commandOK := (in.ModeCommandStatus == "" && in.ModeCommandID == "") || ((in.ModeCommandStatus == "complete" || in.ModeCommandStatus == "failed") && protocol.ValidCommandID(in.ModeCommandID) && len(in.ModeCommandError) <= 200)
	setupOK := (in.MatchSetupStatus == "" && in.MatchSetupID == "") || ((in.MatchSetupStatus == "complete" || in.MatchSetupStatus == "failed") && protocol.ValidCommandID(in.MatchSetupID) && len(in.MatchSetupError) <= 200)
	activeSetupOK := in.ActiveMatchSetupID == "" || (protocol.ValidCommandID(in.ActiveMatchSetupID) && in.Driver == "managed" && in.AccessStatus == "verified" && in.ActivePreset == "match" && (in.MatchPhase == "setup" || in.MatchPhase == "live" || in.MatchPhase == "paused"))
	fenceOK := !in.SupportsMatchFence || (in.Driver == "managed" && in.AccessStatus == "verified" && (in.State == "running" || in.State == "starting"))
	cancelOK := (in.MatchCancelStatus == "" && in.MatchCancelID == "" && in.MatchCancelSetupID == "") || ((in.MatchCancelStatus == "complete" || in.MatchCancelStatus == "failed") && in.Driver == "managed" && in.AccessStatus == "verified" && in.SupportsMatchFence && protocol.ValidCommandID(in.MatchCancelID) && protocol.ValidCommandID(in.MatchCancelSetupID) && len(in.MatchCancelError) <= 200)
	reporterHashOK := (in.ReporterSourceHash == "" || protocol.ValidReporterHash(in.ReporterSourceHash)) && (in.ReporterInstalledHash == "" || protocol.ValidReporterHash(in.ReporterInstalledHash)) && ((in.ReporterSourceHash == "" && in.ReporterInstalledHash == "") || in.Driver == "managed")
	reporterStatusOK := (in.ReporterUpdateStatus == "" && in.ReporterUpdateID == "") || ((in.ReporterUpdateStatus == "complete" || in.ReporterUpdateStatus == "failed") && in.Driver == "managed" && protocol.ValidCommandID(in.ReporterUpdateID) && len(in.ReporterUpdateError) <= 200)
	gameUpdateOK := (in.GameUpdateStatus == "" && in.GameUpdateID == "" && in.GameUpdateError == "") || (in.Driver == "managed" && protocol.ValidCommandID(in.GameUpdateID) && (in.GameUpdateStatus == "pending" || in.GameUpdateStatus == "complete" || in.GameUpdateStatus == "failed") && len(in.GameUpdateError) <= 200)
	matchControlStatusOK := (in.MatchControlStatus == "" && in.MatchControlID == "" && in.MatchControlAction == "") || ((in.MatchControlStatus == "complete" || in.MatchControlStatus == "failed") && in.Driver == "managed" && in.AccessStatus == "verified" && protocol.ValidCommandID(in.MatchControlID) && (in.MatchControlAction == "pause" || in.MatchControlAction == "resume" || in.MatchControlAction == "end") && len(in.MatchControlError) <= 200)
	buildOK := (in.GameBuildInstalled == "" && in.GameBuildLatest == "" && in.GameBuildCheckedAt == 0) || (in.Driver == "managed" && validBuildID.MatchString(in.GameBuildInstalled) && validBuildID.MatchString(in.GameBuildLatest) && in.GameBuildCheckedAt > 0)
	resultOK := in.MatchResult == nil || (in.Driver == "managed" && in.MatchResult.Valid())
	liveScoreOK := in.LiveScore == nil || (in.Driver == "managed" && in.State == "running" && in.AccessStatus == "verified" && in.ActivePreset == "match" && (in.MatchPhase == "live" || in.MatchPhase == "paused") && in.LiveScore.Valid() && in.LiveScore.SetupID == in.ActiveMatchSetupID && in.LiveScore.Map == in.ActiveMap)
	return (in.WorkshopMaps == nil || (in.Driver == "managed" && protocol.ValidWorkshopMaps(*in.WorkshopMaps))) && restartOK && liveScoreOK && buildOK && resultOK && reporterHashOK && reporterStatusOK && gameUpdateOK && fenceOK && cancelOK && activeSetupOK && presetOK && modeOK && matchCapabilityOK && matchSeriesCapabilityOK && matchPhaseOK && matchControlCapabilityOK && matchControlStatusOK && commandOK && setupOK && hashOK && stateOK && driverOK && mapOK && accessOK && len(in.JoinPassword) <= 128 && len(in.Error) <= 1000 && len(in.LogTail) <= 4096 && countOK(in.Players) && countOK(in.MaxPlayers)
}

func (a *API) machines(ctx context.Context) ([]protocol.Machine, error) {
	rows, err := a.DB.Query(ctx, `SELECT id,name,address,driver,last_seen,storage_total,storage_available,cpu_percent,memory_total,memory_available,(SELECT count(*) FROM hosts WHERE machine_id=machines.id AND removed_at IS NULL) FROM machines ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	machines := []protocol.Machine{}
	for rows.Next() {
		var m protocol.Machine
		var count int
		var total, available, memoryTotal, memoryAvailable *int64
		var cpu *float64
		if err := rows.Scan(&m.ID, &m.Name, &m.Address, &m.Driver, &m.LastSeen, &total, &available, &cpu, &memoryTotal, &memoryAvailable, &count); err != nil {
			return nil, err
		}
		m.Online = m.LastSeen != nil && time.Since(*m.LastSeen) < 20*time.Second
		if m.Online && total != nil && available != nil {
			m.Storage = &protocol.MachineStorage{TotalBytes: *total, AvailableBytes: *available}
		}
		if m.Online && memoryTotal != nil && memoryAvailable != nil {
			m.Resources = &protocol.MachineResources{CPUPercent: cpu, MemoryTotalBytes: *memoryTotal, MemoryAvailableBytes: *memoryAvailable}
		}
		m.CreateServer = m.Online && count < protocol.MaxServers
		machines = append(machines, m)
	}
	return machines, rows.Err()
}

func (a *API) createMachine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	if in.Name == "" || len(in.Name) > 80 || !validAddress(in.Address) {
		fail(w, 400, "Provide a machine name and game hostname/IP")
		return
	}
	m := protocol.Machine{ID: rand.Text(), Name: in.Name, Address: in.Address}
	token := rand.Text() + rand.Text()
	_, err := a.DB.Exec(r.Context(), "INSERT INTO machines (id,name,address,token_hash) VALUES ($1,$2,$3,$4)", m.ID, m.Name, m.Address, digest(token))
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		fail(w, 409, "This game address already has a machine registration")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, map[string]any{"machine": m, "agentToken": token})
}

func (a *API) rotateMachineToken(w http.ResponseWriter, r *http.Request) {
	token := rand.Text() + rand.Text()
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	result, err := tx.Exec(r.Context(), `UPDATE machines SET token_hash=$2,last_seen=NULL WHERE id=$1`, r.PathValue("id"), digest(token))
	if err != nil {
		internal(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		fail(w, http.StatusNotFound, "Machine not found")
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE hosts SET last_seen=NULL WHERE machine_id=$1 AND removed_at IS NULL`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]string{"agentToken": token})
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
		Map  string `json:"map"`
		Port int    `json:"port"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 80 || !protocol.ValidMap(in.Map) || (in.Port != 0 && (in.Port < 1024 || in.Port > 65533)) {
		fail(w, 400, "Provide a server name, supported map and game port between 1024 and 65533, or 0 for automatic allocation")
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var address string
	var seen *time.Time
	err = tx.QueryRow(r.Context(), "SELECT address,last_seen FROM machines WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&address, &seen)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "Machine not found")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if seen == nil || time.Since(*seen) >= 20*time.Second {
		fail(w, 409, "Connect the machine agent before creating servers")
		return
	}
	var count int
	if err = tx.QueryRow(r.Context(), "SELECT count(*) FROM hosts WHERE machine_id=$1 AND removed_at IS NULL", r.PathValue("id")).Scan(&count); err != nil {
		internal(w, err)
		return
	}
	if count >= protocol.MaxServers {
		fail(w, 409, "Machine server limit reached")
		return
	}
	rows, err := tx.Query(r.Context(), "SELECT port FROM hosts WHERE (machine_id=$1 OR lower(address)=lower($2)) AND removed_at IS NULL", r.PathValue("id"), address)
	if err != nil {
		internal(w, err)
		return
	}
	ports := []int{}
	for rows.Next() {
		var p int
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		ports = append(ports, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	available := func(p int) bool {
		for _, used := range ports {
			if p <= used+2 && used <= p+2 {
				return false
			}
		}
		return true
	}
	if in.Port == 0 {
		for p := 27015; p <= 65533; p += 10 {
			if available(p) {
				in.Port = p
				break
			}
		}
	}
	if in.Port == 0 || !available(in.Port) {
		fail(w, 409, "Game, client or TV port overlaps another registered server")
		return
	}
	// Fleet instances have no usable single-instance agent credential.
	h, err := scan(tx.QueryRow(r.Context(), `INSERT INTO hosts (id,name,address,port,token_hash,machine_id,map) VALUES ($1,$2,$3,$4,'',$5,$6) RETURNING `+columns, rand.Text(), in.Name, address, in.Port, r.PathValue("id"), in.Map))
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	respond(w, 201, h)
}

func (a *API) machineHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hash string
	err := a.DB.QueryRow(r.Context(), "SELECT token_hash FROM machines WHERE id=$1", r.PathValue("id")).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "Invalid machine credentials")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(digest(bearer(r))), []byte(hash)) != 1 {
		fail(w, 401, "Invalid machine credentials")
		return
	}
	var in protocol.MachineReport
	if !decodeLimit(w, r, &in, 2097152) {
		return
	}
	if (in.Driver != "managed" && in.Driver != "simulated") || len(in.Servers) > protocol.MaxServers || !in.Storage.Valid() || !in.Resources.Valid() || (in.Driver != "managed" && in.Resources != nil) || (in.Driver != "managed" && in.Storage != nil) || (in.SupportsGameUpdate && in.Driver != "managed") {
		fail(w, 400, "Invalid machine status")
		return
	}
	ids := map[string]bool{}
	for _, server := range in.Servers {
		if ids[server.ID] || server.ID == "" || !validReport(server.Report) || server.Driver != in.Driver || (server.State == "removed" && !in.SupportsRemoval) {
			fail(w, 400, "Invalid or duplicate server status")
			return
		}
		ids[server.ID] = true
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	// Serialize snapshots and creation on the same machine; reject the whole report if any ID is foreign.
	// Recheck under the machine lock so a rotated token cannot commit a late report.
	err = tx.QueryRow(r.Context(), "SELECT token_hash FROM machines WHERE id=$1 FOR UPDATE", r.PathValue("id")).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && subtle.ConstantTimeCompare([]byte(digest(bearer(r))), []byte(hash)) != 1 {
		fail(w, http.StatusUnauthorized, "Invalid machine credentials")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE hosts SET workshop_maps=NULL,workshop_maps_seen=NULL WHERE machine_id=$1 AND removed_at IS NULL`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	var storageTotal, storageAvailable *int64
	if in.Storage != nil {
		storageTotal, storageAvailable = &in.Storage.TotalBytes, &in.Storage.AvailableBytes
	}
	var cpu *float64
	var memoryTotal, memoryAvailable *int64
	if in.Resources != nil {
		cpu = in.Resources.CPUPercent
		memoryTotal = &in.Resources.MemoryTotalBytes
		memoryAvailable = &in.Resources.MemoryAvailableBytes
	}
	if _, err = tx.Exec(r.Context(), "UPDATE machines SET driver=$2,supports_removal=$3,supports_whitelist=$4,supports_presets=$5,supports_live_mode=$6,supports_match_setup=$7,supports_match_control=$8,supports_map_observation=$9,supports_match_setup_expiry=$10,supports_match_cancel=$11,supports_reporter_update=$12,supports_game_update=$13,storage_total=$14,storage_available=$15,cpu_percent=$16,memory_total=$17,memory_available=$18,supports_restart=$19,last_seen=now() WHERE id=$1", r.PathValue("id"), in.Driver, in.SupportsRemoval, in.SupportsWhitelist, in.SupportsPresets, in.SupportsLiveMode, in.SupportsMatchSetup, in.SupportsMatchControl, in.Driver == "managed" && in.SupportsMapObservation, in.Driver == "managed" && in.SupportsMatchSetup && in.SupportsMatchSetupExpiry, in.Driver == "managed" && in.SupportsMatchCancel, in.Driver == "managed" && in.SupportsReporterUpdate, in.Driver == "managed" && in.SupportsGameUpdate, storageTotal, storageAvailable, cpu, memoryTotal, memoryAvailable, in.Driver == "managed" && in.SupportsRestart); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE hosts SET mode_command_status='failed',mode_command_error='MatchZy did not confirm the mode change in time'
		WHERE machine_id=$1 AND mode_command_status='pending' AND mode_command_requested_at<now()-interval '30 seconds'`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE match_rooms m SET load_status='failed',load_error='MatchZy did not confirm the loaded match setup'
		FROM hosts h WHERE m.host_id=h.id AND h.machine_id=$1 AND m.load_status='pending' AND m.load_requested_at<now()-interval '45 seconds'`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE hosts SET match_control_status='failed',match_control_error=CASE WHEN match_control_action='end' THEN 'MatchZy did not confirm that the match ended' ELSE 'MatchZy did not confirm the pause state' END WHERE machine_id=$1 AND match_control_status='pending' AND match_control_requested_at<now()-interval '30 seconds'`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE hosts SET reporter_command_status='failed',reporter_command_error='Reporter update was not confirmed in time' WHERE machine_id=$1 AND reporter_command_status='pending' AND reporter_command_requested_at<now()-interval '2 minutes'`, r.PathValue("id")); err != nil {
		internal(w, err)
		return
	}
	resultAcks := map[string]*protocol.MatchResult{}
	controlSetups := map[string]string{}
	type resultReference struct{ hostID, setupID string }
	resultReferences := []resultReference{}
	for _, server := range in.Servers {
		fencedControl := in.SupportsMatchControl && server.SupportsFencedMatchControl && protocol.ValidCommandID(server.ActiveMatchSetupID)
		if fencedControl {
			controlSetups[server.ID] = server.ActiveMatchSetupID
		}
		if _, err = tx.Exec(r.Context(), `UPDATE hosts SET match_control_status='failed',match_control_error='The active match setup or control capability changed' WHERE id=$1 AND machine_id=$2 AND match_control_status='pending' AND match_control_action IN ('pause','resume') AND (NOT $3::boolean OR match_control_setup_id<>$4)`, server.ID, r.PathValue("id"), fencedControl, server.ActiveMatchSetupID); err != nil {
			internal(w, err)
			return
		}
		var liveScore any
		if server.LiveScore != nil {
			age := time.Since(time.UnixMilli(server.LiveScore.ObservedAt))
			if age >= -time.Second && age < 15*time.Second {
				data, marshalErr := json.Marshal(server.LiveScore)
				if marshalErr != nil {
					internal(w, marshalErr)
					return
				}
				liveScore = string(data)
			}
		}
		var result pgconn.CommandTag
		if server.State == "removed" {
			result, err = tx.Exec(r.Context(), `UPDATE hosts SET removed_at=COALESCE(removed_at,now()),last_seen=now(),live_score=NULL,log_tail='',error='',players=NULL,max_players=NULL,access_status='' WHERE id=$1 AND machine_id=$2 AND remove_requested`, server.ID, r.PathValue("id"))
		} else {
			result, err = tx.Exec(r.Context(), `UPDATE hosts SET observed_state=$3,active_map=$4,driver=$5,error=$6,players=$7,max_players=$8,log_tail=$9,access_status=$10,whitelist_hash=$11,applied_preset=$12,active_preset=$13,can_live_mode=$14,can_load_match=$15,
				match_phase=CASE WHEN $19 AND $16 IN ('idle','setup','live','paused') THEN $16 ELSE 'unknown' END,match_paused=($16='paused' AND $19),can_pause_match=($17 AND $19),can_resume_match=($18 AND $19),
				mode_command_status=CASE WHEN mode_command_id=$20 AND mode_command_status='pending' AND $21 IN ('complete','failed') THEN $21 ELSE mode_command_status END,
				mode_command_error=CASE WHEN mode_command_id=$20 AND mode_command_status='pending' AND $21 IN ('complete','failed') THEN $22 ELSE mode_command_error END,
				active_match_setup_id=$27,supports_match_fence=$28,
                reporter_source_hash=$29,reporter_installed_hash=$30,
                reporter_command_status=CASE WHEN reporter_command_status='pending' AND $3<>'stopped' THEN 'failed' WHEN reporter_command_status='pending' AND reporter_command_id=$31 AND $32='failed' THEN 'failed' WHEN reporter_command_status='pending' AND reporter_command_id=$31 AND $32='complete' AND $30=reporter_requested_hash THEN 'complete' ELSE reporter_command_status END,
                reporter_command_error=CASE WHEN reporter_command_status='pending' AND $3<>'stopped' THEN 'Server started before the reporter update completed' WHEN reporter_command_status='pending' AND reporter_command_id=$31 AND $32='failed' THEN $33 WHEN reporter_command_status='pending' AND reporter_command_id=$31 AND $32='complete' AND $30=reporter_requested_hash THEN '' ELSE reporter_command_error END,
				match_control_status=CASE WHEN match_control_id=$23 AND match_control_action=$24 AND match_control_status='pending' AND $25 IN ('complete','failed') THEN $25 ELSE match_control_status END,
				match_control_error=CASE WHEN match_control_id=$23 AND match_control_action=$24 AND match_control_status='pending' AND $25 IN ('complete','failed') THEN $26 ELSE match_control_error END,
				live_score=$34::jsonb,supports_manual_ready=$35,last_seen=now() WHERE id=$1 AND machine_id=$2 AND removed_at IS NULL`, server.ID, r.PathValue("id"), server.State, server.ActiveMap, server.Driver, server.Error, server.Players, server.MaxPlayers, server.LogTail, server.AccessStatus, server.WhitelistHash, server.AppliedPreset, server.ActivePreset, in.SupportsLiveMode && server.CanLiveMode, in.SupportsMatchSetup && server.CanLoadMatch, server.MatchPhase, fencedControl && server.CanPauseMatch, fencedControl && server.CanResumeMatch, in.SupportsMatchControl, server.ModeCommandID, server.ModeCommandStatus, server.ModeCommandError, server.MatchControlID, server.MatchControlAction, server.MatchControlStatus, server.MatchControlError, server.ActiveMatchSetupID, in.SupportsMatchCancel && server.SupportsMatchFence, reporterHash(in.SupportsReporterUpdate, server.ReporterSourceHash), reporterHash(in.SupportsReporterUpdate, server.ReporterInstalledHash), server.ReporterUpdateID, server.ReporterUpdateStatus, server.ReporterUpdateError, liveScore, in.Driver == "managed" && server.SupportsManualReady)
		}
		if err != nil {
			internal(w, err)
			return
		}
		if result.RowsAffected() != 1 {
			fail(w, 400, "Server does not belong to this machine")
			return
		}
		if server.State != "removed" {
			if server.WorkshopMaps != nil {
				if _, err = tx.Exec(r.Context(), `UPDATE hosts SET workshop_maps=$3,workshop_maps_seen=clock_timestamp() WHERE id=$1 AND machine_id=$2`, server.ID, r.PathValue("id"), *server.WorkshopMaps); err != nil {
					internal(w, err)
					return
				}
			}
			if err = acknowledgeRestart(r.Context(), tx, server.ID, r.PathValue("id"), in.Driver == "managed" && in.SupportsRestart, server.Report); err != nil {
				internal(w, err)
				return
			}
		}
		if _, err = tx.Exec(r.Context(), `UPDATE hosts SET supports_match_series=$3 WHERE id=$1 AND machine_id=$2`, server.ID, r.PathValue("id"), server.SupportsMatchSeries); err != nil {
			internal(w, err)
			return
		}
		if server.State != "removed" {
			var checkedAt *time.Time
			if server.GameBuildCheckedAt > time.Now().Add(-24*time.Hour).UnixMilli() && server.GameBuildCheckedAt < time.Now().Add(5*time.Minute).UnixMilli() {
				t := time.UnixMilli(server.GameBuildCheckedAt)
				checkedAt = &t
			} else {
				server.GameBuildInstalled = ""
				server.GameBuildLatest = ""
			}
			if _, err = tx.Exec(r.Context(), `UPDATE hosts SET game_build_installed=$3,game_build_latest=$4,game_build_checked_at=$5 WHERE id=$1 AND machine_id=$2`, server.ID, r.PathValue("id"), server.GameBuildInstalled, server.GameBuildLatest, checkedAt); err != nil {
				internal(w, err)
				return
			}
		}
		if server.GameUpdateID != "" {
			if _, err = tx.Exec(r.Context(), `UPDATE game_updates SET status=$3,error=$4,updated_at=now() WHERE host_id=$1 AND request_id=$2 AND status='pending'`, server.ID, server.GameUpdateID, server.GameUpdateStatus, server.GameUpdateError); err != nil {
				internal(w, err)
				return
			}
		}
		if result := server.MatchResult; result != nil {
			// The first accepted observation wins; later retries must match it exactly.
			payload, err := json.Marshal(result)
			if err != nil {
				internal(w, err)
				return
			}
			ids := make([]string, 0, len(result.Players))
			for _, p := range result.Players {
				ids = append(ids, p.SteamID)
			}
			var matchID string
			err = tx.QueryRow(r.Context(), `UPDATE match_rooms SET reported_ct=$3,reported_t=$4,reported_team1_score=$7::smallint,reported_team2_score=$8::smallint,reported_at=now(),reported_payload=$5::jsonb,result_revision=result_revision+1,
				status='closed',closed_at=COALESCE(closed_at,now()),load_status='complete',load_error='',load_retry_after=NULL,
				cancel_status=CASE WHEN cancel_status='pending' THEN 'failed' ELSE cancel_status END,
				cancel_error=CASE WHEN cancel_status='pending' THEN 'The match finished before cancellation was confirmed' ELSE cancel_error END
				WHERE host_id=$1 AND load_command_id=$2 AND load_fence_required AND load_status IN ('pending','failed','complete') AND reported_ct IS NULL
				AND (($7::smallint IS NULL AND $8::smallint IS NULL) OR (jsonb_array_length(load_payload->'maps') IN (3,5) AND EXISTS (SELECT 1 FROM tournament_matches tm JOIN tournaments t ON t.id=tm.tournament_id WHERE tm.room_id=match_rooms.id AND t.best_of=jsonb_array_length(match_rooms.load_payload->'maps'))))
				AND NOT EXISTS (SELECT unnest($6::text[]) EXCEPT SELECT steam_id FROM match_room_players WHERE match_id=match_rooms.id) RETURNING id`, server.ID, result.SetupID, result.CT, result.T, string(payload), ids, result.Team1Score, result.Team2Score).Scan(&matchID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				internal(w, err)
				return
			}
			if err == nil {
				for _, p := range result.Players {
					if _, err = tx.Exec(r.Context(), `UPDATE match_room_players SET reported_kills=$3,reported_deaths=$4,reported_assists=$5 WHERE match_id=$1 AND steam_id=$2`, matchID, p.SteamID, p.Kills, p.Deaths, p.Assists); err != nil {
						internal(w, err)
						return
					}
				}
			}
			var accepted bool
			if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND load_command_id=$2 AND load_fence_required AND reported_ct=$3 AND reported_t=$4 AND reported_team1_score IS NOT DISTINCT FROM $7 AND reported_team2_score IS NOT DISTINCT FROM $8 AND (reported_payload=$5::jsonb OR (reported_payload IS NULL AND $6=0)))`, server.ID, result.SetupID, result.CT, result.T, string(payload), len(result.Players), result.Team1Score, result.Team2Score).Scan(&accepted); err != nil {
				internal(w, err)
				return
			}
			if accepted {
				resultAcks[server.ID] = result
				resultReferences = append(resultReferences, resultReference{hostID: server.ID, setupID: result.SetupID})
			}
		}
		if server.MatchControlAction == "end" && server.MatchControlStatus == "complete" {
			if _, err = tx.Exec(r.Context(), `UPDATE match_rooms m SET status='closed',closed_at=COALESCE(closed_at,now()),ended_at=COALESCE(ended_at,now()) FROM hosts h WHERE h.id=$1 AND m.host_id=h.id AND h.match_control_id=$2 AND h.match_control_action='end' AND h.match_control_status='complete' AND m.load_command_id=h.match_control_setup_id AND m.load_status='complete'`, server.ID, server.MatchControlID); err != nil {
				internal(w, err)
				return
			}
		}
		if server.MatchSetupStatus == "complete" || server.MatchSetupStatus == "failed" {
			if _, err = tx.Exec(r.Context(), `UPDATE match_rooms SET load_status=$3,load_error=$4 WHERE host_id=$1 AND load_command_id=$2 AND load_status='pending'`, server.ID, server.MatchSetupID, server.MatchSetupStatus, server.MatchSetupError); err != nil {
				internal(w, err)
				return
			}
		}
		// Runtime evidence can supersede a timeout even if the plugin result still says failed.
		if in.SupportsMatchSetup && in.SupportsMatchControl && server.State == "running" && server.Driver == "managed" && server.AccessStatus == "verified" && server.ActivePreset == "match" && !server.CanLoadMatch && protocol.ValidCommandID(server.ActiveMatchSetupID) {
			if _, err = tx.Exec(r.Context(), `UPDATE match_rooms SET load_status='complete',load_error='',load_retry_after=NULL,cancel_status=CASE WHEN cancel_status='pending' THEN 'failed' ELSE cancel_status END,cancel_error=CASE WHEN cancel_status='pending' THEN 'The setup became active before cancellation' ELSE cancel_error END WHERE host_id=$1 AND load_command_id=$2 AND load_status='failed'`, server.ID, server.ActiveMatchSetupID); err != nil {
				internal(w, err)
				return
			}
		}
		if in.SupportsMatchCancel && server.SupportsMatchFence && server.MatchCancelStatus != "" {
			if server.MatchCancelStatus == "failed" {
				if _, err = tx.Exec(r.Context(), `UPDATE match_rooms SET cancel_status='failed',cancel_error=$4 WHERE host_id=$1 AND cancel_id=$2 AND load_command_id=$3 AND load_status='failed' AND cancel_status='pending'`, server.ID, server.MatchCancelID, server.MatchCancelSetupID, server.MatchCancelError); err != nil {
					internal(w, err)
					return
				}
			} else if server.State == "running" && server.AccessStatus == "verified" && server.ActivePreset == "match" && server.MatchPhase == "idle" && server.CanLoadMatch && server.ActiveMatchSetupID == "" {
				var recoveredID string
				err = tx.QueryRow(r.Context(), `UPDATE match_rooms SET load_status='',load_error='',load_command_id='',load_requested_at=NULL,load_payload=NULL,load_retry_after=NULL,load_expiry_enforced=false,load_fence_required=false,cancel_status='complete',cancel_error='' WHERE host_id=$1 AND cancel_id=$2 AND load_command_id=$3 AND load_status='failed' AND cancel_status='pending' AND load_fence_required AND load_expiry_enforced RETURNING id`, server.ID, server.MatchCancelID, server.MatchCancelSetupID).Scan(&recoveredID)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					internal(w, err)
					return
				}
				if err == nil {
					if _, err = tx.Exec(r.Context(), "UPDATE match_room_players SET ready=false WHERE match_id=$1 AND ready=true", recoveredID); err != nil {
						internal(w, err)
						return
					}
				}
			}
		}
	}
	rows, err := tx.Query(r.Context(), "SELECT id,desired_state,map,port,remove_requested,whitelist,preset,mode_command_id,COALESCE(mode_command_mode,''),mode_command_status,match_control_id,COALESCE(match_control_action,''),match_control_status,match_control_setup_id,reporter_command_id,reporter_requested_hash,reporter_command_status,COALESCE((SELECT request_id FROM game_updates WHERE host_id=hosts.id AND status='pending' AND hosts.desired_state='stopped' AND hosts.observed_state='stopped'),'') FROM hosts WHERE machine_id=$1 AND removed_at IS NULL ORDER BY created_at,id", r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	out := protocol.MachineDesired{SupportsResultStats: true, Servers: []protocol.ServerDesired{}}
	for rows.Next() {
		var s protocol.ServerDesired
		var whitelist []string
		var commandID, commandMode, commandStatus, controlID, controlAction, controlStatus, controlSetupID, reporterID, reporterHashValue, reporterStatus, gameUpdateID string
		if err = rows.Scan(&s.ID, &s.State, &s.Map, &s.Port, &s.Remove, &whitelist, &s.Preset, &commandID, &commandMode, &commandStatus, &controlID, &controlAction, &controlStatus, &controlSetupID, &reporterID, &reporterHashValue, &reporterStatus, &gameUpdateID); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		if whitelist != nil {
			if in.SupportsWhitelist {
				s.Whitelist = &whitelist
			} else {
				s.State = "stopped"
			}
		}
		if s.Preset != nil && !in.SupportsPresets {
			s.Preset = nil
			s.State = "stopped"
		}
		if commandStatus == "pending" && s.State == "running" && in.SupportsLiveMode && protocol.ValidCommandID(commandID) && (commandMode == "match" || commandMode == "practice") {
			s.LiveMode = &protocol.ModeCommand{ID: commandID, Mode: commandMode}
		}
		if reporterStatus == "pending" && s.State == "stopped" && in.SupportsReporterUpdate && protocol.ValidCommandID(reporterID) && protocol.ValidReporterHash(reporterHashValue) {
			s.ReporterUpdate = &protocol.ReporterUpdate{ID: reporterID, Hash: reporterHashValue}
		}
		if in.SupportsGameUpdate && in.Driver == "managed" && s.State == "stopped" && protocol.ValidCommandID(gameUpdateID) {
			s.GameUpdate = &protocol.GameUpdate{ID: gameUpdateID}
		}
		if controlStatus == "pending" && s.State == "running" && in.SupportsMatchControl && protocol.ValidCommandID(controlID) && protocol.ValidCommandID(controlSetupID) && (controlAction == "end" || ((controlAction == "pause" || controlAction == "resume") && controlSetups[s.ID] == controlSetupID)) {
			s.MatchControl = &protocol.MatchControl{ID: controlID, Action: controlAction, SetupID: controlSetupID}
		}
		s.ResultAck = resultAcks[s.ID]
		out.Servers = append(out.Servers, s)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if in.SupportsMatchCancel && in.Driver == "managed" {
		for i := range out.Servers {
			s := &out.Servers[i]
			var command protocol.MatchCancel
			err = tx.QueryRow(r.Context(), `SELECT m.cancel_id,m.load_command_id FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.host_id=$1 AND m.load_status='failed' AND m.cancel_status='pending' AND m.load_fence_required AND h.supports_match_fence AND $2='running'`, s.ID, s.State).Scan(&command.ID, &command.SetupID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				internal(w, err)
				return
			}
			if protocol.ValidCommandID(command.ID) && protocol.ValidCommandID(command.SetupID) {
				s.MatchCancel = &command
			}
		}
	}
	if in.SupportsMatchSetup && in.SupportsMapObservation && in.Driver == "managed" {
		for i := range out.Servers {
			s := &out.Servers[i]
			var setup protocol.MatchSetup
			var payload []byte
			var commandID, mapName string
			var playersPerSide int
			var expiryEnforced, fenceRequired, currentFence bool
			err = tx.QueryRow(r.Context(), `SELECT m.load_payload,m.load_command_id,m.map,m.players_per_side,m.load_expiry_enforced,m.load_fence_required,h.supports_match_fence FROM match_rooms m JOIN hosts h ON h.id=m.host_id WHERE m.host_id=$1 AND m.load_status='pending' AND m.load_payload IS NOT NULL AND m.load_requested_at>now()-interval '45 seconds' AND $2='running' AND h.active_map<>'' AND h.match_control_status<>'pending' AND h.mode_command_status<>'pending' AND (NOT m.manual_ready OR h.supports_manual_ready) ORDER BY m.load_requested_at LIMIT 1`, s.ID, s.State).Scan(&payload, &commandID, &mapName, &playersPerSide, &expiryEnforced, &fenceRequired, &currentFence)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				internal(w, err)
				return
			}
			if (expiryEnforced && !in.SupportsMatchSetupExpiry) || (fenceRequired && (!in.SupportsMatchCancel || !currentFence)) {
				continue
			}
			if err = json.Unmarshal(payload, &setup); err != nil || setup.ID != commandID || !protocol.ValidCommandID(setup.ID) || (expiryEnforced && setup.RequestedAt <= 0) || setup.FenceRequired != fenceRequired || setup.Map != mapName || !protocol.ValidInstalledMap(setup.Map) || setup.PlayersPerSide != playersPerSide || playersPerSide < 1 || playersPerSide > 5 || len(setup.CT) < 1 || len(setup.CT) > playersPerSide || len(setup.T) < 1 || len(setup.T) > playersPerSide {
				if _, err = tx.Exec(r.Context(), `UPDATE match_rooms SET load_status='failed',load_error='Saved match setup is invalid; check the room and retry' WHERE host_id=$1 AND load_command_id=$2 AND load_status='pending'`, s.ID, commandID); err != nil {
					internal(w, err)
					return
				}
				continue
			}
			h, err := lockHost(r.Context(), tx, s.ID)
			if err != nil {
				internal(w, err)
				return
			}
			maps := append([]string{setup.Map}, setup.Maps...)
			available := true
			for _, mapName := range maps {
				available = available && hostHasMap(h, mapName)
			}
			if !available {
				if _, err = tx.Exec(r.Context(), `UPDATE match_rooms SET load_status='failed',load_error='Map is no longer available on this server' WHERE host_id=$1 AND load_command_id=$2 AND load_status='pending'`, s.ID, commandID); err != nil {
					internal(w, err)
					return
				}
				continue
			}
			s.MatchSetup = &setup
		}
	}
	for i := range out.Servers {
		s := &out.Servers[i]
		var id, phase string
		err = tx.QueryRow(r.Context(), `SELECT request_id,phase FROM server_restarts WHERE host_id=$1 AND phase IN ('stopping','starting','cancelling')`, s.ID).Scan(&id, &phase)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			internal(w, err)
			return
		}
		// No other agent operation can accompany a restart, even after a downgrade.
		s.GameUpdate, s.ReporterUpdate, s.LiveMode, s.MatchSetup, s.MatchControl, s.MatchCancel = nil, nil, nil, nil, nil, nil
		if !in.SupportsRestart || in.Driver != "managed" || !protocol.ValidCommandID(id) {
			s.State = "stopped"
			continue
		}
		switch {
		case phase == "stopping" && s.State == "stopped":
			s.Restart = &protocol.RestartCommand{ID: id, Phase: "stop"}
		case phase == "starting" && s.State == "running":
			s.Restart = &protocol.RestartCommand{ID: id, Phase: "start"}
		case phase == "cancelling" && s.State == "stopped":
			s.Restart = &protocol.RestartCommand{ID: id, Phase: "cancel"}
		default:
			s.State = "stopped"
		}
	}
	for i := range out.Servers {
		s := &out.Servers[i]
		h, err := lockHost(r.Context(), tx, s.ID)
		if err != nil {
			internal(w, err)
			return
		}
		if !hostHasMap(h, s.Map) {
			s.State = "stopped"
			s.Restart, s.MatchSetup, s.LiveMode, s.MatchControl = nil, nil, nil, nil
			s.Map = protocol.Maps[0]
		}
	}
	// Commit the issuance fence before a response can reach the agent. Cancellation
	// takes the same host lock, so the two operations cannot both succeed.
	for i := range out.Servers {
		s := &out.Servers[i]
		if s.GameUpdate == nil {
			continue
		}
		var desired, observed string
		err = tx.QueryRow(r.Context(), `SELECT desired_state,observed_state FROM hosts WHERE id=$1 AND machine_id=$2 AND removed_at IS NULL FOR UPDATE`, s.ID, r.PathValue("id")).Scan(&desired, &observed)
		if errors.Is(err, pgx.ErrNoRows) {
			s.GameUpdate = nil
			continue
		}
		if err != nil {
			internal(w, err)
			return
		}
		if desired != "stopped" || observed != "stopped" {
			s.GameUpdate = nil
			continue
		}
		var issued bool
		err = tx.QueryRow(r.Context(), `SELECT issued_at IS NOT NULL FROM game_updates WHERE host_id=$1 AND request_id=$2 AND status='pending'`, s.ID, s.GameUpdate.ID).Scan(&issued)
		if errors.Is(err, pgx.ErrNoRows) {
			s.GameUpdate = nil
			continue
		}
		if err != nil {
			internal(w, err)
			return
		}
		if !issued {
			if _, err = tx.Exec(r.Context(), `UPDATE game_updates SET issued_at=now(),updated_at=now() WHERE host_id=$1 AND request_id=$2 AND status='pending'`, s.ID, s.GameUpdate.ID); err != nil {
				internal(w, err)
				return
			}
			if _, err = tx.Exec(r.Context(), `UPDATE hosts SET revision=revision+1 WHERE id=$1`, s.ID); err != nil {
				internal(w, err)
				return
			}
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	for _, reference := range resultReferences {
		if err = a.advanceTournamentFromReportedResult(r.Context(), reference.hostID, reference.setupID); err != nil {
			internal(w, err)
			return
		}
	}
	for _, server := range in.Servers {
		if in.Driver == "managed" && server.State != "removed" {
			a.rememberJoinPassword(server.ID, server.JoinPassword)
		} else {
			a.rememberJoinPassword(server.ID, "")
		}
	}
	respond(w, 200, out)
}

func reporterHash(supported bool, hash string) string {
	if supported {
		return hash
	}
	return ""
}
