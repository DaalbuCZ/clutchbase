ALTER TABLE machines ADD COLUMN supports_live_mode boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN can_live_mode boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN mode_command_id text NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN mode_command_mode text CHECK (mode_command_mode IN ('match','practice'));
ALTER TABLE hosts ADD COLUMN mode_command_status text NOT NULL DEFAULT '' CHECK (mode_command_status IN ('','pending','complete','failed','cancelled'));
ALTER TABLE hosts ADD COLUMN mode_command_error text NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN mode_command_requested_at timestamptz;
