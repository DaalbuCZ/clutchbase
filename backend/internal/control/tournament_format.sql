ALTER TABLE tournaments ADD COLUMN format text NOT NULL DEFAULT 'single_elimination' CHECK (format IN ('single_elimination','round_robin'));
