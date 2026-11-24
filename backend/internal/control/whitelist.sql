ALTER TABLE hosts ADD COLUMN whitelist text[];
ALTER TABLE hosts ADD COLUMN whitelist_hash text NOT NULL DEFAULT '';
ALTER TABLE machines ADD COLUMN supports_whitelist boolean NOT NULL DEFAULT false;
