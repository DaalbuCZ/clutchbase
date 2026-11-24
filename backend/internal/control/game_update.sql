ALTER TABLE machines ADD COLUMN supports_game_update boolean NOT NULL DEFAULT false;
CREATE TABLE game_updates (
    host_id text PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    request_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','complete','failed')),
    error text NOT NULL DEFAULT '',
    requested_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
