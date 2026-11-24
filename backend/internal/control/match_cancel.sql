ALTER TABLE machines ADD COLUMN supports_match_cancel boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN supports_match_fence boolean NOT NULL DEFAULT false;
ALTER TABLE match_rooms
    ADD COLUMN load_fence_required boolean NOT NULL DEFAULT false,
    ADD COLUMN cancel_id text NOT NULL DEFAULT '',
    ADD COLUMN cancel_status text NOT NULL DEFAULT '' CHECK (cancel_status IN ('','pending','complete','failed')),
    ADD COLUMN cancel_error text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX one_pending_match_cancel_per_host ON match_rooms(host_id) WHERE cancel_status='pending';
