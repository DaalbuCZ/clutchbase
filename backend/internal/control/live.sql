CREATE FUNCTION notify_workspace_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('clutchbase_state', TG_TABLE_SCHEMA);
    RETURN NULL;
END;
$$;
CREATE TRIGGER hosts_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON hosts
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
CREATE TRIGGER machines_workspace_changed AFTER INSERT OR UPDATE OR DELETE ON machines
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
