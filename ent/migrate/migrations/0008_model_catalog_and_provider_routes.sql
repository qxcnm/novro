-- Novro migration 0008: decouple the reusable model catalog from provider credentials.
ALTER TABLE upstream_models
    ADD COLUMN provider_name VARCHAR(128) NOT NULL DEFAULT '' AFTER id,
    ADD COLUMN pricing_configured BOOLEAN NOT NULL DEFAULT TRUE AFTER request_price_micros;

UPDATE upstream_models AS catalog_model
JOIN providers AS configured_provider ON configured_provider.id = catalog_model.provider_id
SET catalog_model.provider_name = configured_provider.display_name
WHERE catalog_model.provider_name = '';

ALTER TABLE upstream_models
    DROP FOREIGN KEY upstream_models_providers_upstream_models,
    DROP INDEX upstream_models_provider_id_upstream_name,
    DROP INDEX upstream_models_provider_id_status,
    MODIFY COLUMN provider_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    ADD UNIQUE KEY upstream_models_provider_name_upstream_name (provider_name, upstream_name),
    ADD KEY upstream_models_provider_name_status (provider_name, status);
