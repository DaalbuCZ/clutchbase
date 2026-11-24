ALTER TABLE match_rooms ADD COLUMN reported_ct integer, ADD COLUMN reported_t integer, ADD COLUMN reported_at timestamptz,
    ADD CONSTRAINT match_reported_score_check CHECK (
      (reported_ct IS NULL AND reported_t IS NULL AND reported_at IS NULL) OR
      (reported_ct IS NOT NULL AND reported_t IS NOT NULL AND reported_at IS NOT NULL AND reported_ct BETWEEN 0 AND 99 AND reported_t BETWEEN 0 AND 99)
    );
