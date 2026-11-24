ALTER TABLE match_rooms ADD COLUMN load_payload jsonb,
    ADD COLUMN load_retry_after timestamptz;

-- Earlier versions could resend different player names under the same command ID.
-- Do not replay an in-flight request whose original payload was not retained.
UPDATE match_rooms
SET load_status='failed', load_error='Match setup was interrupted by an upgrade; wait one minute and check the server before retrying',
    load_retry_after=now()+interval '60 seconds'
WHERE load_status='pending';
