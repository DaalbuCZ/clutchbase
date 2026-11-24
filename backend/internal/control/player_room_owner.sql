ALTER TABLE match_rooms ADD COLUMN created_by_steam_id text REFERENCES players(steam_id) ON DELETE SET NULL;
