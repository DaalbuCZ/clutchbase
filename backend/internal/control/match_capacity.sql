ALTER TABLE match_rooms
    ADD COLUMN players_per_side smallint NOT NULL DEFAULT 5
    CHECK (players_per_side BETWEEN 1 AND 5);
