ALTER TABLE match_rooms ADD COLUMN manual_ready boolean NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN supports_manual_ready boolean NOT NULL DEFAULT false;
