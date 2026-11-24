CREATE TABLE tournament_disputes (
    match_id text PRIMARY KEY REFERENCES tournament_matches(id) ON DELETE CASCADE,
    reason text NOT NULL,
    resolution text NOT NULL DEFAULT '',
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0)
);
CREATE TRIGGER tournament_disputes_changed AFTER INSERT OR UPDATE OR DELETE ON tournament_disputes
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
