-- Novro migration 0009: seed current popular DeepSeek, GLM, and Kimi models.
-- Price snapshot: 2026-08-06, RMB micros per 1M tokens.
-- Official sources:
-- https://api-docs.deepseek.com/zh-cn/quick_start/pricing/
-- https://bigmodel.cn/pricing
-- https://platform.kimi.com/docs/pricing/chat-k3
-- https://platform.kimi.com/docs/pricing/chat-k27-code
-- https://platform.kimi.com/docs/pricing/chat-k26
-- GLM models with length-tiered prices are intentionally not seeded because a
-- single flat rate cannot represent their official billing rules accurately.

CREATE TEMPORARY TABLE novro_catalog_provider_aliases (
    alias_name VARCHAR(128) NOT NULL PRIMARY KEY,
    canonical_name VARCHAR(128) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

START TRANSACTION;

INSERT IGNORE INTO novro_catalog_provider_aliases (alias_name, canonical_name)
SELECT display_name, 'DeepSeek'
FROM providers
WHERE LOWER(base_url) = 'https://api.deepseek.com'
   OR LOWER(base_url) LIKE 'https://api.deepseek.com/%';

INSERT IGNORE INTO novro_catalog_provider_aliases (alias_name, canonical_name)
SELECT display_name, '智谱 GLM'
FROM providers
WHERE LOWER(base_url) = 'https://open.bigmodel.cn'
   OR LOWER(base_url) LIKE 'https://open.bigmodel.cn/%';

INSERT IGNORE INTO novro_catalog_provider_aliases (alias_name, canonical_name)
SELECT display_name, 'Kimi'
FROM providers
WHERE LOWER(base_url) = 'https://api.moonshot.cn'
   OR LOWER(base_url) LIKE 'https://api.moonshot.cn/%';

-- Copy models discovered under account display names into the reusable,
-- canonical provider catalog before repointing existing references.
INSERT INTO upstream_models (
    id, provider_name, upstream_name, display_name,
    input_price_micros, output_price_micros, cache_read_price_micros,
    cache_write_price_micros, cache_write_1h_price_micros, request_price_micros,
    pricing_configured, status, created_at, updated_at
)
SELECT
    UUID(), aliases.canonical_name, models.upstream_name, MAX(models.display_name),
    MAX(models.input_price_micros), MAX(models.output_price_micros), MAX(models.cache_read_price_micros),
    MAX(models.cache_write_price_micros), MAX(models.cache_write_1h_price_micros), MAX(models.request_price_micros),
    MAX(models.pricing_configured),
    IF(MAX(models.status = 'active') = 1, 'active', 'disabled'),
    MIN(models.created_at), MAX(models.updated_at)
FROM upstream_models AS models
JOIN novro_catalog_provider_aliases AS aliases ON aliases.alias_name = models.provider_name
WHERE aliases.alias_name <> aliases.canonical_name
GROUP BY aliases.canonical_name, models.upstream_name
ON DUPLICATE KEY UPDATE upstream_name = upstream_models.upstream_name;

UPDATE model_routes AS routes
JOIN upstream_models AS duplicate_model ON duplicate_model.id = routes.upstream_model_id
JOIN novro_catalog_provider_aliases AS aliases
  ON aliases.alias_name = duplicate_model.provider_name
 AND aliases.alias_name <> aliases.canonical_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = aliases.canonical_name
 AND canonical_model.upstream_name = duplicate_model.upstream_name
SET routes.upstream_model_id = canonical_model.id;

UPDATE api_usages AS usage_record
JOIN upstream_models AS duplicate_model ON duplicate_model.id = usage_record.upstream_model_id
JOIN novro_catalog_provider_aliases AS aliases
  ON aliases.alias_name = duplicate_model.provider_name
 AND aliases.alias_name <> aliases.canonical_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = aliases.canonical_name
 AND canonical_model.upstream_name = duplicate_model.upstream_name
SET usage_record.upstream_model_id = canonical_model.id;

DELETE duplicate_model
FROM upstream_models AS duplicate_model
JOIN novro_catalog_provider_aliases AS aliases
  ON aliases.alias_name = duplicate_model.provider_name
 AND aliases.alias_name <> aliases.canonical_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = aliases.canonical_name
 AND canonical_model.upstream_name = duplicate_model.upstream_name;

DROP TEMPORARY TABLE novro_catalog_provider_aliases;

INSERT INTO upstream_models (
    id, provider_name, upstream_name, display_name,
    input_price_micros, output_price_micros, cache_read_price_micros,
    cache_write_price_micros, cache_write_1h_price_micros, request_price_micros,
    pricing_configured, status, created_at, updated_at
) VALUES
    ('10000000-0000-0000-0000-000000000001', 'DeepSeek', 'deepseek-v4-flash', 'DeepSeek V4 Flash', 1000000, 2000000, 20000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('10000000-0000-0000-0000-000000000002', 'DeepSeek', 'deepseek-v4-pro', 'DeepSeek V4 Pro', 3000000, 6000000, 25000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('20000000-0000-0000-0000-000000000001', '智谱 GLM', 'glm-5.2', 'GLM-5.2', 8000000, 28000000, 2000000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('20000000-0000-0000-0000-000000000002', '智谱 GLM', 'glm-4.7-flashx', 'GLM-4.7 FlashX', 500000, 3000000, 100000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('30000000-0000-0000-0000-000000000001', 'Kimi', 'kimi-k3', 'Kimi K3', 20000000, 100000000, 2000000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('30000000-0000-0000-0000-000000000002', 'Kimi', 'kimi-k2.7-code', 'Kimi K2.7 Code', 6500000, 27000000, 1300000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('30000000-0000-0000-0000-000000000003', 'Kimi', 'kimi-k2.7-code-highspeed', 'Kimi K2.7 Code HighSpeed', 13000000, 54000000, 2600000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6)),
    ('30000000-0000-0000-0000-000000000004', 'Kimi', 'kimi-k2.6', 'Kimi K2.6', 6500000, 27000000, 1100000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE
    display_name = VALUES(display_name),
    input_price_micros = VALUES(input_price_micros),
    output_price_micros = VALUES(output_price_micros),
    cache_read_price_micros = VALUES(cache_read_price_micros),
    cache_write_price_micros = VALUES(cache_write_price_micros),
    cache_write_1h_price_micros = VALUES(cache_write_1h_price_micros),
    request_price_micros = VALUES(request_price_micros),
    pricing_configured = TRUE,
    status = 'active',
    updated_at = UTC_TIMESTAMP(6);

CREATE TEMPORARY TABLE novro_seed_catalog_keys (
    provider_name VARCHAR(128) NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    PRIMARY KEY (provider_name, upstream_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO novro_seed_catalog_keys (provider_name, upstream_name) VALUES
    ('DeepSeek', 'deepseek-v4-flash'),
    ('DeepSeek', 'deepseek-v4-pro'),
    ('智谱 GLM', 'glm-5.2'),
    ('智谱 GLM', 'glm-4.7-flashx'),
    ('Kimi', 'kimi-k3'),
    ('Kimi', 'kimi-k2.7-code'),
    ('Kimi', 'kimi-k2.7-code-highspeed'),
    ('Kimi', 'kimi-k2.6');

UPDATE model_routes AS routes
JOIN upstream_models AS duplicate_model ON duplicate_model.id = routes.upstream_model_id
JOIN novro_seed_catalog_keys AS seed_key ON seed_key.upstream_name = duplicate_model.upstream_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = seed_key.provider_name
 AND canonical_model.upstream_name = seed_key.upstream_name
SET routes.upstream_model_id = canonical_model.id
WHERE duplicate_model.id <> canonical_model.id;

UPDATE api_usages AS usage_record
JOIN upstream_models AS duplicate_model ON duplicate_model.id = usage_record.upstream_model_id
JOIN novro_seed_catalog_keys AS seed_key ON seed_key.upstream_name = duplicate_model.upstream_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = seed_key.provider_name
 AND canonical_model.upstream_name = seed_key.upstream_name
SET usage_record.upstream_model_id = canonical_model.id
WHERE duplicate_model.id <> canonical_model.id;

DELETE duplicate_model
FROM upstream_models AS duplicate_model
JOIN novro_seed_catalog_keys AS seed_key ON seed_key.upstream_name = duplicate_model.upstream_name
JOIN upstream_models AS canonical_model
  ON canonical_model.provider_name = seed_key.provider_name
 AND canonical_model.upstream_name = seed_key.upstream_name
WHERE duplicate_model.id <> canonical_model.id;

DROP TEMPORARY TABLE novro_seed_catalog_keys;

COMMIT;
