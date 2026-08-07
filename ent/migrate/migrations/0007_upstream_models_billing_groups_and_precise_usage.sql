-- Novro migration 0007: upstream model catalogs, billing groups, and immutable pricing snapshots.
CREATE TABLE IF NOT EXISTS billing_groups (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    code VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    multiplier_bps BIGINT NOT NULL DEFAULT 10000,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY billing_groups_code_key (code),
    KEY billing_groups_status_created_at (status, created_at),
    KEY billing_groups_is_default (is_default)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO billing_groups (id, code, display_name, multiplier_bps, is_default, status, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'default', '默认分组', 10000, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE updated_at = updated_at;

ALTER TABLE users
    ADD COLUMN billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER id,
    ADD KEY users_billing_group_id (billing_group_id),
    ADD CONSTRAINT users_billing_groups_users
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION;

UPDATE users
SET billing_group_id = '00000000-0000-0000-0000-000000000001'
WHERE billing_group_id IS NULL;

CREATE TABLE IF NOT EXISTS upstream_models (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    provider_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    input_price_micros BIGINT NOT NULL DEFAULT 0,
    output_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_read_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0,
    request_price_micros BIGINT NOT NULL DEFAULT 0,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY upstream_models_provider_id_upstream_name (provider_id, upstream_name),
    KEY upstream_models_provider_id_status (provider_id, status),
    KEY upstream_models_status_created_at (status, created_at),
    CONSTRAINT upstream_models_providers_upstream_models
        FOREIGN KEY (provider_id) REFERENCES providers (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO upstream_models (
    id, provider_id, upstream_name, display_name, input_price_micros, output_price_micros,
    cache_read_price_micros, cache_write_price_micros, cache_write_1h_price_micros,
    request_price_micros, status, created_at, updated_at
)
SELECT UUID(), provider_id, upstream_name, MAX(display_name), MAX(input_price_micros), MAX(output_price_micros),
       0, 0, 0, 0, MAX(status), MIN(created_at), MAX(updated_at)
FROM model_routes
GROUP BY provider_id, upstream_name
ON DUPLICATE KEY UPDATE upstream_name = upstream_name;

ALTER TABLE model_routes
    ADD COLUMN upstream_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER provider_id,
    ADD KEY model_routes_upstream_model_id_status (upstream_model_id, status),
    ADD CONSTRAINT model_routes_upstream_models_model_routes
        FOREIGN KEY (upstream_model_id) REFERENCES upstream_models (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION;

UPDATE model_routes AS routes
JOIN upstream_models AS upstream
  ON upstream.provider_id = routes.provider_id AND upstream.upstream_name = routes.upstream_name
SET routes.upstream_model_id = upstream.id
WHERE routes.upstream_model_id IS NULL;

ALTER TABLE api_usages
    ADD COLUMN upstream_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER model_route_id,
    ADD COLUMN billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER upstream_model_id,
    ADD COLUMN uncached_input_tokens INT NOT NULL DEFAULT 0 AFTER input_tokens,
    ADD COLUMN cache_read_input_tokens INT NOT NULL DEFAULT 0 AFTER uncached_input_tokens,
    ADD COLUMN cache_write_input_tokens INT NOT NULL DEFAULT 0 AFTER cache_read_input_tokens,
    ADD COLUMN cache_write_1h_input_tokens INT NOT NULL DEFAULT 0 AFTER cache_write_input_tokens,
    ADD COLUMN input_price_micros BIGINT NOT NULL DEFAULT 0 AFTER output_tokens,
    ADD COLUMN output_price_micros BIGINT NOT NULL DEFAULT 0 AFTER input_price_micros,
    ADD COLUMN cache_read_price_micros BIGINT NOT NULL DEFAULT 0 AFTER output_price_micros,
    ADD COLUMN cache_write_price_micros BIGINT NOT NULL DEFAULT 0 AFTER cache_read_price_micros,
    ADD COLUMN cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0 AFTER cache_write_price_micros,
    ADD COLUMN request_price_micros BIGINT NOT NULL DEFAULT 0 AFTER cache_write_1h_price_micros,
    ADD COLUMN base_cost_micros BIGINT NOT NULL DEFAULT 0 AFTER request_price_micros,
    ADD COLUMN multiplier_bps BIGINT NOT NULL DEFAULT 10000 AFTER base_cost_micros,
    ADD COLUMN model_name VARCHAR(128) NOT NULL DEFAULT '' AFTER upstream_request_id,
    ADD COLUMN upstream_model_name VARCHAR(256) NOT NULL DEFAULT '' AFTER model_name,
    ADD COLUMN billing_group_code VARCHAR(64) NOT NULL DEFAULT '' AFTER upstream_model_name,
    ADD COLUMN billing_group_name VARCHAR(128) NOT NULL DEFAULT '' AFTER billing_group_code,
    ADD COLUMN calculation_version VARCHAR(32) NOT NULL DEFAULT 'v1' AFTER billing_group_name,
    ADD KEY api_usages_upstream_model_id_created_at (upstream_model_id, created_at),
    ADD KEY api_usages_billing_group_id_created_at (billing_group_id, created_at),
    ADD CONSTRAINT api_usages_upstream_models_api_usages
        FOREIGN KEY (upstream_model_id) REFERENCES upstream_models (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    ADD CONSTRAINT api_usages_billing_groups_api_usages
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION;

UPDATE api_usages AS api_usage
JOIN model_routes AS routes ON routes.id = api_usage.model_route_id
JOIN upstream_models AS upstream ON upstream.id = routes.upstream_model_id
JOIN users AS owner ON owner.id = api_usage.user_id
JOIN billing_groups AS billing_group ON billing_group.id = owner.billing_group_id
SET api_usage.upstream_model_id = upstream.id,
    api_usage.billing_group_id = billing_group.id,
    api_usage.uncached_input_tokens = api_usage.input_tokens,
    api_usage.input_price_micros = routes.input_price_micros,
    api_usage.output_price_micros = routes.output_price_micros,
    api_usage.base_cost_micros = api_usage.cost_micros,
    api_usage.model_name = routes.public_name,
    api_usage.upstream_model_name = upstream.upstream_name,
    api_usage.billing_group_code = billing_group.code,
    api_usage.billing_group_name = billing_group.display_name,
    api_usage.calculation_version = 'legacy-v0'
WHERE api_usage.upstream_model_id IS NULL;

ALTER TABLE wallet_entries
    MODIFY COLUMN entry_type ENUM('manual_adjustment', 'usage_reservation', 'usage_refund', 'usage_settlement') NOT NULL;
