ALTER TABLE match_rooms ADD COLUMN pause_vote_threshold integer NOT NULL DEFAULT 1 CHECK (pause_vote_threshold BETWEEN 1 AND 10);
CREATE TABLE match_pause_votes (
    match_id text NOT NULL,
    steam_id text NOT NULL,
    PRIMARY KEY (match_id, steam_id),
    FOREIGN KEY (match_id, steam_id) REFERENCES match_room_players(match_id, steam_id) ON DELETE CASCADE
);
CREATE TRIGGER match_pause_votes_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_pause_votes
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
CREATE FUNCTION clear_pause_votes_on_match_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.match_phase IS DISTINCT FROM NEW.match_phase OR OLD.active_match_setup_id IS DISTINCT FROM NEW.active_match_setup_id THEN
        DELETE FROM match_pause_votes WHERE match_id IN (SELECT id FROM match_rooms WHERE host_id=NEW.id);
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER host_clear_pause_votes AFTER UPDATE OF match_phase, active_match_setup_id ON hosts
FOR EACH ROW EXECUTE FUNCTION clear_pause_votes_on_match_change();
