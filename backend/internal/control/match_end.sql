ALTER TABLE hosts DROP CONSTRAINT hosts_match_control_action_check;
ALTER TABLE hosts ADD CONSTRAINT hosts_match_control_action_check CHECK (match_control_action IN ('pause','resume','end'));
ALTER TABLE hosts ADD COLUMN active_match_setup_id text NOT NULL DEFAULT '', ADD COLUMN match_control_setup_id text NOT NULL DEFAULT '';
ALTER TABLE match_rooms ADD COLUMN ended_at timestamptz;
