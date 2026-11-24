ALTER TABLE match_room_players ADD COLUMN kills integer, ADD COLUMN deaths integer, ADD COLUMN assists integer,
    ADD CONSTRAINT manual_stats_check CHECK (
      (kills IS NULL AND deaths IS NULL AND assists IS NULL) OR
      (kills IS NOT NULL AND deaths IS NOT NULL AND assists IS NOT NULL AND
       kills BETWEEN 0 AND 999 AND deaths BETWEEN 0 AND 999 AND assists BETWEEN 0 AND 999)
    );
