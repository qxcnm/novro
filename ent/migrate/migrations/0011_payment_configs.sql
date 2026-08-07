-- Novro migration 0011: administrator-managed payment configuration.
CREATE TABLE IF NOT EXISTS payment_configs (
    provider VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    api_url VARCHAR(512) NOT NULL DEFAULT '',
    merchant_id VARCHAR(128) NOT NULL DEFAULT '',
    encrypted_merchant_key VARCHAR(2048) NOT NULL DEFAULT '',
    site_name VARCHAR(64) NOT NULL DEFAULT 'Novro',
    channels VARCHAR(512) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (provider)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
