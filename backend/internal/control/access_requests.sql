CREATE TABLE access_requests (
    id text PRIMARY KEY,
    host_id text NOT NULL REFERENCES hosts(id),
    steam_id text NOT NULL REFERENCES players(steam_id),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','rejected','approved')),
    revision bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(host_id, steam_id)
);
CREATE INDEX access_requests_player_idx ON access_requests(steam_id, created_at, id);
CREATE INDEX access_requests_host_idx ON access_requests(host_id, created_at, id);
CREATE TRIGGER access_requests_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON access_requests
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
