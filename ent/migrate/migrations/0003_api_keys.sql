-- Novro migration 0003: user API keys.
CREATE TABLE IF NOT EXISTS api_keys (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    name VARCHAR(64) NOT NULL,
    key_prefix VARCHAR(16) NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    status ENUM('active', 'revoked') NOT NULL DEFAULT 'active',
    last_used_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY api_keys_key_hash_key (key_hash),
    KEY api_keys_user_id_status (user_id, status),
    KEY api_keys_status_created_at (status, created_at),
    CONSTRAINT api_keys_users_api_keys
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
