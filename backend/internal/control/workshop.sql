ALTER TABLE hosts ADD COLUMN workshop_maps text[];
ALTER TABLE hosts ADD COLUMN workshop_maps_seen timestamptz;
ALTER TABLE match_rooms DROP CONSTRAINT match_rooms_map_check;
ALTER TABLE match_rooms ADD CONSTRAINT match_rooms_map_check CHECK (
 map IN ('de_mirage','de_inferno','de_dust2','de_nuke','de_ancient','de_anubis','de_train','de_vertigo')
 OR map ~ '^workshop/[1-9][0-9]{0,19}/[a-z0-9_-]{1,128}$');
