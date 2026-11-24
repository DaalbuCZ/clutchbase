CREATE TABLE players (
    steam_id text PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE player_sessions (
    token_hash text PRIMARY KEY,
    steam_id text NOT NULL REFERENCES players(steam_id),
    expires_at timestamptz NOT NULL
);
CREATE INDEX player_session_expiry ON player_sessions(expires_at);
CREATE TABLE steam_logins (
    state_hash text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE TRIGGER player_sessions_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON player_sessions
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
