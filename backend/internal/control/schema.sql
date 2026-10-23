CREATE TABLE IF NOT EXISTS hosts (
    id text PRIMARY KEY,
    name text NOT NULL,
    address text NOT NULL,
    port integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
    token_hash text NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    map text NOT NULL DEFAULT 'de_mirage',
    desired_state text NOT NULL DEFAULT 'stopped' CHECK (desired_state IN ('running', 'stopped')),
    observed_state text NOT NULL DEFAULT 'unknown',
    active_map text NOT NULL DEFAULT '',
    driver text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    last_seen timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
