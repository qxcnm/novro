-- Novro migration 0027: remove seeded catalog rows that retained dynamic IDs.
-- Migration 0009 used ON DUPLICATE KEY UPDATE, so an earlier synchronized row
-- could keep its original UUID while receiving the built-in model price card.
-- Match the exact 0009 seed keys, preserve history with soft deletion, and let
-- a later provider sync restore only models that are still advertised.

CREATE TEMPORARY TABLE novro_legacy_seed_catalog_keys (
    provider_name VARCHAR(128) NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    PRIMARY KEY (provider_name, upstream_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO novro_legacy_seed_catalog_keys (provider_name, upstream_name) VALUES
    ('DeepSeek', 'deepseek-v4-flash'),
    ('DeepSeek', 'deepseek-v4-pro'),
    ('智谱 GLM', 'glm-5.2'),
    ('智谱 GLM', 'glm-4.7-flashx'),
    ('Kimi', 'kimi-k3'),
    ('Kimi', 'kimi-k2.7-code'),
    ('Kimi', 'kimi-k2.7-code-highspeed'),
    ('Kimi', 'kimi-k2.6');

UPDATE model_routes AS route
JOIN upstream_models AS catalog_model ON catalog_model.id = route.upstream_model_id
JOIN novro_legacy_seed_catalog_keys AS seed_key
  ON seed_key.provider_name = catalog_model.provider_name
 AND seed_key.upstream_name = catalog_model.upstream_name
SET route.status = 'disabled',
    route.deleted_at = COALESCE(route.deleted_at, UTC_TIMESTAMP(6)),
    route.updated_at = UTC_TIMESTAMP(6);

UPDATE upstream_models AS catalog_model
JOIN novro_legacy_seed_catalog_keys AS seed_key
  ON seed_key.provider_name = catalog_model.provider_name
 AND seed_key.upstream_name = catalog_model.upstream_name
SET catalog_model.status = 'disabled',
    catalog_model.deleted_at = COALESCE(catalog_model.deleted_at, UTC_TIMESTAMP(6)),
    catalog_model.updated_at = UTC_TIMESTAMP(6);

DROP TEMPORARY TABLE novro_legacy_seed_catalog_keys;
