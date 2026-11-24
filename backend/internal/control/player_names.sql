ALTER TABLE players ADD COLUMN game_name text NOT NULL DEFAULT '';
CREATE TRIGGER players_profile_workspace_changed AFTER UPDATE OF game_name ON players
FOR EACH STATEMENT EXECUTE FUNCTION notify_workspace_change();
