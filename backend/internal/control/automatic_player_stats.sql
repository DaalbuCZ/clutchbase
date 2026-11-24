ALTER TABLE match_room_players ADD COLUMN reported_kills integer, ADD COLUMN reported_deaths integer, ADD COLUMN reported_assists integer,
    ADD CONSTRAINT reported_stats_check CHECK (
      (reported_kills IS NULL AND reported_deaths IS NULL AND reported_assists IS NULL) OR
      (reported_kills IS NOT NULL AND reported_deaths IS NOT NULL AND reported_assists IS NOT NULL AND
       reported_kills BETWEEN 0 AND 999 AND reported_deaths BETWEEN 0 AND 999 AND reported_assists BETWEEN 0 AND 999)
    );
ALTER TABLE match_rooms ADD COLUMN reported_payload jsonb;
