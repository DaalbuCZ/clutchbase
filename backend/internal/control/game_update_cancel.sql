ALTER TABLE game_updates ADD COLUMN issued_at timestamptz;
-- Existing pending requests may already have reached an agent.
UPDATE game_updates SET issued_at=now() WHERE status='pending';
ALTER TABLE game_updates DROP CONSTRAINT game_updates_status_check;
ALTER TABLE game_updates ADD CONSTRAINT game_updates_status_check CHECK (status IN ('pending','complete','failed','cancelled'));
