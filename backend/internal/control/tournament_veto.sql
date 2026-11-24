ALTER TABLE tournaments
    ADD COLUMN map_pool text[] NOT NULL DEFAULT ARRAY['de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train']::text[],
    ADD CONSTRAINT tournaments_map_pool_size CHECK (cardinality(map_pool)=7);

ALTER TABLE tournament_matches
    ADD COLUMN veto_bans text[] NOT NULL DEFAULT ARRAY[]::text[],
    ADD COLUMN veto_step smallint NOT NULL DEFAULT 0 CHECK (veto_step BETWEEN 0 AND 7),
    ADD COLUMN veto_turn_team_id text REFERENCES teams(id),
    ADD COLUMN selected_map text CHECK (selected_map IS NULL OR selected_map IN ('de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train','de_vertigo')),
    ADD COLUMN team_b_side text CHECK (team_b_side IS NULL OR team_b_side IN ('ct','t'));
