CREATE TABLE server_restarts (
    host_id text PRIMARY KEY REFERENCES hosts(id),
    request_id text NOT NULL,
    phase text NOT NULL CHECK (phase IN ('stopping','starting','cancelling','complete','cancelled','failed')),
    requested_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER server_restarts_state_changed AFTER INSERT OR UPDATE OR DELETE ON server_restarts
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
