ALTER TABLE machines ADD COLUMN supports_reporter_update boolean NOT NULL DEFAULT false;
ALTER TABLE hosts
    ADD COLUMN reporter_source_hash text NOT NULL DEFAULT '',
    ADD COLUMN reporter_installed_hash text NOT NULL DEFAULT '',
    ADD COLUMN reporter_command_id text NOT NULL DEFAULT '',
    ADD COLUMN reporter_requested_hash text NOT NULL DEFAULT '',
    ADD COLUMN reporter_command_status text NOT NULL DEFAULT '' CHECK (reporter_command_status IN ('','pending','complete','failed')),
    ADD COLUMN reporter_command_error text NOT NULL DEFAULT '',
    ADD COLUMN reporter_command_requested_at timestamptz;
