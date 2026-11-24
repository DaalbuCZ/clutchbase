CREATE TABLE match_series_map_results (
    match_id text NOT NULL REFERENCES match_rooms(id) ON DELETE CASCADE,
    map_number smallint NOT NULL CHECK (map_number BETWEEN 1 AND 5),
    team1_score smallint NOT NULL CHECK (team1_score BETWEEN 0 AND 99),
    team2_score smallint NOT NULL CHECK (team2_score BETWEEN 0 AND 99),
    PRIMARY KEY (match_id, map_number)
);
CREATE TRIGGER series_map_results_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON match_series_map_results
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
