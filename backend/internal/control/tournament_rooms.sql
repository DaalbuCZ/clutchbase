ALTER TABLE tournament_matches
    ADD COLUMN room_id text REFERENCES match_rooms(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX tournament_match_room_unique ON tournament_matches(room_id) WHERE room_id IS NOT NULL;

CREATE OR REPLACE FUNCTION notify_clutchbase_tournament_room_state() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('clutchbase_state', 'changed');
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tournament_match_room_state_changed
AFTER UPDATE OF room_id ON tournament_matches
FOR EACH STATEMENT EXECUTE FUNCTION notify_clutchbase_tournament_room_state();
