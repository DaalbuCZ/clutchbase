ALTER TABLE tournaments DROP CONSTRAINT tournaments_status_check;
ALTER TABLE tournaments ADD CONSTRAINT tournaments_status_check CHECK (status IN ('open','checkin','active','complete','cancelled'));
