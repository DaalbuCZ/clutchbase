CREATE TABLE match_rooms (
    id text PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    host_id text NOT NULL REFERENCES hosts(id) ON DELETE RESTRICT,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 80),
    map text NOT NULL CHECK (map IN ('de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train','de_vertigo')),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    closed_at timestamptz
);
CREATE UNIQUE INDEX one_open_match_room_per_host ON match_rooms(host_id) WHERE status='open';
CREATE TABLE match_room_players (
    match_id text NOT NULL REFERENCES match_rooms(id) ON DELETE CASCADE,
    steam_id text NOT NULL REFERENCES players(steam_id) ON DELETE CASCADE,
    ready boolean NOT NULL DEFAULT false,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (match_id, steam_id)
);
CREATE INDEX match_room_players_by_player ON match_room_players(steam_id, match_id);
CREATE TRIGGER match_rooms_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_rooms
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
CREATE TRIGGER match_room_players_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_room_players
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
