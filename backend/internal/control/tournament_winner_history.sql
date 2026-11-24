CREATE TABLE tournament_winner_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    match_id text NOT NULL REFERENCES tournament_matches(id) ON DELETE CASCADE,
    revision bigint NOT NULL CHECK (revision > 0),
    previous_winner text,
    new_winner text,
    changed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_id, revision)
);

CREATE FUNCTION record_tournament_winner_change() RETURNS trigger AS $$
BEGIN
    IF NEW.winner_team_id IS DISTINCT FROM OLD.winner_team_id THEN
        INSERT INTO tournament_winner_history(match_id, revision, previous_winner, new_winner)
        VALUES (NEW.id, NEW.result_revision, OLD.winner_team_id, NEW.winner_team_id);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tournament_winner_history_after_update
AFTER UPDATE ON tournament_matches FOR EACH ROW EXECUTE FUNCTION record_tournament_winner_change();
