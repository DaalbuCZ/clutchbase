ALTER TABLE teams ADD COLUMN revision bigint NOT NULL DEFAULT 0, ADD COLUMN disbanded_at timestamptz;
