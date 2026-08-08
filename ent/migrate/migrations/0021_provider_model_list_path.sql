-- Novro migration 0021: configurable provider model discovery path.
ALTER TABLE providers ADD COLUMN model_list_path VARCHAR(512) NOT NULL DEFAULT '' AFTER base_url;
