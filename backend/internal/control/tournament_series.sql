ALTER TABLE tournaments
    ADD COLUMN best_of smallint NOT NULL DEFAULT 1 CHECK (best_of IN (1,3,5));

ALTER TABLE tournament_matches
    ADD COLUMN veto_maps text[] NOT NULL DEFAULT ARRAY[]::text[],
    ADD COLUMN map_sequence text[] NOT NULL DEFAULT ARRAY[]::text[];

ALTER TABLE match_rooms
    ADD COLUMN series_maps text[] NOT NULL DEFAULT ARRAY[]::text[],
    ADD COLUMN series_sides text[] NOT NULL DEFAULT ARRAY[]::text[],
    ADD COLUMN reported_team1_score smallint CHECK (reported_team1_score BETWEEN 0 AND 5),
    ADD COLUMN reported_team2_score smallint CHECK (reported_team2_score BETWEEN 0 AND 5),
    ADD CONSTRAINT match_rooms_reported_series_pair CHECK ((reported_team1_score IS NULL) = (reported_team2_score IS NULL));
