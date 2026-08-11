-- Novro migration 0002: add deterministic provider priority.

ALTER TABLE providers
    ADD COLUMN weight INT NOT NULL DEFAULT 100 AFTER model_list_path;
