CREATE TABLE IF NOT EXISTS model_price_plans (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    upstream_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    version INT NOT NULL,
    mode ENUM('fixed', 'scheduled') NOT NULL DEFAULT 'fixed',
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    effective_from DATETIME(6) NOT NULL,
    effective_to DATETIME(6) NULL,
    status ENUM('draft', 'published', 'retired') NOT NULL DEFAULT 'draft',
    default_input_price_micros BIGINT NOT NULL DEFAULT 0,
    default_output_price_micros BIGINT NOT NULL DEFAULT 0,
    default_cache_read_price_micros BIGINT NOT NULL DEFAULT 0,
    default_cache_write_price_micros BIGINT NOT NULL DEFAULT 0,
    default_cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0,
    default_request_price_micros BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY model_price_plan_model_version (upstream_model_id, version),
    KEY model_price_plan_model_status_effective (upstream_model_id, status, effective_from),
    KEY model_price_plan_status_effective (status, effective_from),
    CONSTRAINT model_price_plans_upstream_models
        FOREIGN KEY (upstream_model_id) REFERENCES upstream_models (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS model_price_windows (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    price_plan_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    label VARCHAR(64) NOT NULL,
    weekday_mask INT NOT NULL,
    start_minute INT NOT NULL,
    end_minute INT NOT NULL,
    input_price_micros BIGINT NOT NULL DEFAULT 0,
    output_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_read_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0,
    request_price_micros BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    KEY model_price_window_plan_weekday_start (price_plan_id, weekday_mask, start_minute),
    CONSTRAINT model_price_windows_model_price_plans
        FOREIGN KEY (price_plan_id) REFERENCES model_price_plans (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO model_price_plans (
    id, upstream_model_id, version, mode, timezone, effective_from, effective_to, status,
    default_input_price_micros, default_output_price_micros,
    default_cache_read_price_micros, default_cache_write_price_micros,
    default_cache_write_1h_price_micros, default_request_price_micros,
    created_at, updated_at
)
SELECT
    id, id, 1, 'fixed', 'Asia/Shanghai', '1970-01-01 00:00:00.000000', NULL,
    CASE
        WHEN deleted_at IS NOT NULL THEN 'retired'
        WHEN pricing_configured = TRUE THEN 'published'
        ELSE 'draft'
    END,
    input_price_micros, output_price_micros, cache_read_price_micros,
    cache_write_price_micros, cache_write_1h_price_micros, request_price_micros,
    created_at, updated_at
FROM upstream_models
WHERE NOT EXISTS (
    SELECT 1 FROM model_price_plans existing
    WHERE existing.upstream_model_id = upstream_models.id AND existing.version = 1
);
