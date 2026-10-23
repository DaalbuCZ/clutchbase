CREATE TABLE machines (
    id text PRIMARY KEY,
    name text NOT NULL,
    address text NOT NULL,
    token_hash text NOT NULL,
    driver text NOT NULL DEFAULT '',
    last_seen timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX machines_address ON machines(lower(address));
ALTER TABLE hosts ADD COLUMN machine_id text REFERENCES machines(id);
CREATE INDEX hosts_machine_id ON hosts(machine_id);
