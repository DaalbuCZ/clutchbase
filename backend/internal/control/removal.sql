ALTER TABLE machines ADD COLUMN supports_removal boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN remove_requested boolean NOT NULL DEFAULT false;
-- Keep a tombstone so a lost acknowledgement can be safely retried.
ALTER TABLE hosts ADD COLUMN removed_at timestamptz;
