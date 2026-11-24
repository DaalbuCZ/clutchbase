ALTER TABLE hosts ADD COLUMN game_build_installed text NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN game_build_latest text NOT NULL DEFAULT '';
ALTER TABLE hosts ADD COLUMN game_build_checked_at timestamptz;
