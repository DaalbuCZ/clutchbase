ALTER TABLE match_rooms
    ADD COLUMN load_command_id text NOT NULL DEFAULT '',
    ADD COLUMN load_status text NOT NULL DEFAULT '' CHECK (load_status IN ('','pending','complete','failed')),
    ADD COLUMN load_error text NOT NULL DEFAULT '',
    ADD COLUMN load_requested_at timestamptz;
ALTER TABLE machines ADD COLUMN supports_match_setup boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN can_load_match boolean NOT NULL DEFAULT false;
