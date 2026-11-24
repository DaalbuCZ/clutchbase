ALTER TABLE match_room_players ADD COLUMN side text NOT NULL DEFAULT '' CHECK (side IN ('','ct','t'));
