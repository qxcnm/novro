-- Novro migration 0004: encrypted model provider configuration.
CREATE TABLE IF NOT EXISTS providers (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    code VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    protocol ENUM('openai', 'anthropic') NOT NULL,
    base_url VARCHAR(512) NOT NULL,
    encrypted_api_key VARCHAR(2048) NOT NULL,
    api_key_hint VARCHAR(8) NOT NULL,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY providers_code_key (code),
    KEY providers_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
