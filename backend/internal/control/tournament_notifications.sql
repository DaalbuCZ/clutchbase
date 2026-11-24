CREATE OR REPLACE FUNCTION notify_clutchbase_tournament_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('clutchbase_state', TG_TABLE_SCHEMA);
    RETURN NULL;
END;
$$;
CREATE OR REPLACE FUNCTION notify_clutchbase_tournament_room_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM pg_notify('clutchbase_state', TG_TABLE_SCHEMA);
    RETURN NULL;
END;
$$;
