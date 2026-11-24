ALTER TABLE tournament_matches ADD COLUMN scheduled_at timestamptz;
ALTER TABLE tournament_matches ADD COLUMN schedule_revision bigint NOT NULL DEFAULT 0 CHECK (schedule_revision >= 0);
