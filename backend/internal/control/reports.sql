CREATE TABLE match_reports (
    id text PRIMARY KEY,
    match_id text NOT NULL REFERENCES match_rooms(id),
    reporter_steam_id text NOT NULL REFERENCES players(steam_id),
    description text NOT NULL CHECK (length(description) BETWEEN 1 AND 1000),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_id, reporter_steam_id)
);
CREATE TRIGGER match_reports_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_reports
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
