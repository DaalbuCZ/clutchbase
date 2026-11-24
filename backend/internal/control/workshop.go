package control

import (
	"clutchbase/internal/protocol"
	"slices"
)

func hostHasMap(h protocol.Host, name string) bool { return slices.Contains(h.AvailableMaps, name) }

// Reporter 0.3.15 cannot load or identify Workshop maps; discovery alone is insufficient.
const roomMapAvailableSQL = `(m.map NOT LIKE 'workshop/%' AND NOT EXISTS (SELECT 1 FROM unnest(m.series_maps) AS sm(map) WHERE sm.map LIKE 'workshop/%'))`

// Keep the snapshot and locked mutation's server load gate identical.
const matchLoadEligibleSQL = `COALESCE((m.status='open' AND m.load_status='' AND NOT EXISTS (SELECT 1 FROM match_rooms previous WHERE previous.host_id=h.id AND previous.id<>m.id AND previous.load_status='failed')
 AND h.last_seen>now()-interval '20 seconds' AND h.desired_state='running' AND h.observed_state='running'
 AND h.driver='managed' AND h.active_preset='match' AND h.access_status='verified' AND COALESCE(h.error,'')=''
 AND h.removed_at IS NULL AND NOT h.remove_requested AND h.match_phase='idle' AND h.active_map<>'' AND COALESCE(h.active_match_setup_id,'')=''
 AND ` + roomMapAvailableSQL + ` AND h.can_load_match AND (NOT m.manual_ready OR h.supports_manual_ready)
 AND (cardinality(m.series_maps)<=1 OR h.supports_match_series)
 AND COALESCE((SELECT supports_map_observation FROM machines WHERE id=h.machine_id),false)
 AND COALESCE(h.match_control_status,'')<>'pending' AND COALESCE(h.mode_command_status,'')<>'pending'),false)`

// Blockers name all failed gates above plus roster conditions checked under lock.
const matchLoadBlockersSQL = `CASE WHEN m.status<>'open' OR m.load_status<>'' THEN ARRAY[]::text[] ELSE array_remove(ARRAY[
 CASE WHEN m.map LIKE 'workshop/%' OR EXISTS(SELECT 1 FROM unnest(m.series_maps) sm(map) WHERE sm.map LIKE 'workshop/%') THEN 'workshop_unsupported' END,
 CASE WHEN COALESCE(h.last_seen<=now()-interval '20 seconds',true) OR h.desired_state<>'running' OR h.observed_state<>'running' THEN 'server_offline' END,
 CASE WHEN COALESCE(h.access_status,'')<>'verified' THEN 'server_access_unverified' END,
 CASE WHEN NOT h.can_load_match OR (m.manual_ready AND NOT h.supports_manual_ready) OR (cardinality(m.series_maps)>1 AND NOT h.supports_match_series) OR NOT COALESCE((SELECT supports_map_observation FROM machines WHERE id=h.machine_id),false) THEN 'reporter_unsupported' END,
 CASE WHEN h.removed_at IS NOT NULL OR COALESCE(h.driver,'')<>'managed' OR COALESCE(h.active_preset,'')<>'match' OR COALESCE(h.active_map,'')='' OR COALESCE(h.match_phase,'')<>'idle' OR COALESCE(h.error,'')<>'' OR h.remove_requested OR COALESCE(h.active_match_setup_id,'')<>'' OR COALESCE(h.match_control_status,'')='pending' OR COALESCE(h.mode_command_status,'')='pending' OR EXISTS(SELECT 1 FROM match_rooms previous WHERE previous.host_id=h.id AND previous.id<>m.id AND previous.load_status='failed') THEN 'server_not_ready' END,
 CASE WHEN count(p.steam_id)=0 OR count(p.steam_id) FILTER(WHERE p.side='ct')=0 OR count(p.steam_id) FILTER(WHERE p.side='t')=0 OR count(p.steam_id) FILTER(WHERE p.side='ct')>m.players_per_side OR count(p.steam_id) FILTER(WHERE p.side='t')>m.players_per_side OR count(p.ready) FILTER(WHERE p.ready)<count(p.steam_id) THEN 'players_not_ready' END
]::text[],NULL) END`
