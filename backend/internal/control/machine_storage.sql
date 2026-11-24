ALTER TABLE machines ADD COLUMN storage_total bigint, ADD COLUMN storage_available bigint,
    ADD CONSTRAINT machine_storage_check CHECK (
      (storage_total IS NULL AND storage_available IS NULL) OR
      (storage_total IS NOT NULL AND storage_available IS NOT NULL AND storage_total > 0 AND storage_available BETWEEN 0 AND storage_total)
    );
