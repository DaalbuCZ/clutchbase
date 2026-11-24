ALTER TABLE hosts ADD COLUMN players integer;
ALTER TABLE hosts ADD COLUMN max_players integer;
ALTER TABLE hosts ADD COLUMN log_tail text NOT NULL DEFAULT '';
