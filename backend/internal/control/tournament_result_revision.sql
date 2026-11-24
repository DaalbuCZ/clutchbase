ALTER TABLE tournament_matches ADD COLUMN result_revision bigint NOT NULL DEFAULT 0 CHECK (result_revision >= 0);

CREATE FUNCTION bump_tournament_result_revision() RETURNS trigger AS $$
BEGIN
    IF (NEW.team_a, NEW.team_b, NEW.winner_team_id, NEW.status)
       IS DISTINCT FROM (OLD.team_a, OLD.team_b, OLD.winner_team_id, OLD.status) THEN
        NEW.result_revision := OLD.result_revision + 1;
    ELSE
        NEW.result_revision := OLD.result_revision;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER tournament_result_revision_before_update
BEFORE UPDATE ON tournament_matches FOR EACH ROW EXECUTE FUNCTION bump_tournament_result_revision();
