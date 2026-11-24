ALTER TABLE machines ADD COLUMN supports_match_control boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN match_phase text NOT NULL DEFAULT 'unknown' CHECK (match_phase IN ('unknown','idle','setup','live','paused')),
    ADD COLUMN match_paused boolean NOT NULL DEFAULT false,
    ADD COLUMN can_pause_match boolean NOT NULL DEFAULT false,
    ADD COLUMN can_resume_match boolean NOT NULL DEFAULT false,
    ADD COLUMN match_control_id text NOT NULL DEFAULT '',
    ADD COLUMN match_control_action text CHECK (match_control_action IN ('pause','resume')),
    ADD COLUMN match_control_status text NOT NULL DEFAULT '' CHECK (match_control_status IN ('','pending','complete','failed')),
    ADD COLUMN match_control_error text NOT NULL DEFAULT '',
    ADD COLUMN match_control_requested_at timestamptz;
