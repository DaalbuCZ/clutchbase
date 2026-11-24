ALTER TABLE match_rooms
    ADD COLUMN result_ct integer,
    ADD COLUMN result_t integer,
    ADD COLUMN result_revision bigint NOT NULL DEFAULT 0,
    ADD CONSTRAINT match_result_scores_check CHECK (
        (result_ct IS NULL AND result_t IS NULL) OR
        (result_ct IS NOT NULL AND result_t IS NOT NULL AND result_ct BETWEEN 0 AND 99 AND result_t BETWEEN 0 AND 99)
    );
