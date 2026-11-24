ALTER TABLE hosts ADD COLUMN preset text CHECK (preset IN ('','match','practice'));
ALTER TABLE hosts ADD COLUMN applied_preset text CHECK (applied_preset IN ('','match','practice'));
ALTER TABLE hosts ADD COLUMN active_preset text NOT NULL DEFAULT '' CHECK (active_preset IN ('','match','practice'));
ALTER TABLE machines ADD COLUMN supports_presets boolean NOT NULL DEFAULT false;
