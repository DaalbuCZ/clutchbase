ALTER TABLE machines ADD COLUMN supports_match_setup_expiry boolean NOT NULL DEFAULT false;
ALTER TABLE match_rooms ADD COLUMN load_expiry_enforced boolean NOT NULL DEFAULT false;
