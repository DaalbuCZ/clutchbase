CREATE TABLE match_map_votes (
    match_id text NOT NULL,
    steam_id text NOT NULL,
    map text NOT NULL CHECK (map IN ('de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train','de_vertigo')),
    PRIMARY KEY (match_id,steam_id),
    FOREIGN KEY (match_id,steam_id) REFERENCES match_room_players(match_id,steam_id) ON DELETE CASCADE
);
CREATE TRIGGER match_map_votes_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_map_votes
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
