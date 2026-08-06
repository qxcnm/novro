-- Novro migration 0005: model routing, wallet ledger, and metered API usage.
CREATE TABLE IF NOT EXISTS model_routes (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    provider_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    public_name VARCHAR(128) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    input_price_micros BIGINT NOT NULL,
    output_price_micros BIGINT NOT NULL,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY model_routes_public_name_key (public_name),
    KEY model_routes_provider_id_status (provider_id, status),
    KEY model_routes_status_created_at (status, created_at),
    CONSTRAINT model_routes_providers_model_routes
        FOREIGN KEY (provider_id) REFERENCES providers (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS wallets (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    balance_micros BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY wallets_user_id_key (user_id),
    CONSTRAINT wallets_users_wallet
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT IGNORE INTO wallets (id, user_id, balance_micros, created_at, updated_at)
SELECT UUID(), id, 0, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6) FROM users;

CREATE TABLE IF NOT EXISTS wallet_entries (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    wallet_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    actor_user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    reference_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    entry_type ENUM('manual_adjustment', 'usage_reservation', 'usage_refund') NOT NULL,
    amount_micros BIGINT NOT NULL,
    balance_after_micros BIGINT NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    KEY wallet_entries_wallet_id_created_at (wallet_id, created_at),
    KEY wallet_entries_reference_id (reference_id),
    KEY wallet_entries_actor_user_id (actor_user_id),
    CONSTRAINT wallet_entries_wallets_entries
        FOREIGN KEY (wallet_id) REFERENCES wallets (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT wallet_entries_users_wallet_entries
        FOREIGN KEY (actor_user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS api_usages (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    api_key_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    model_route_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    request_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    endpoint ENUM('chat_completions', 'responses', 'messages') NOT NULL,
    input_tokens INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    cost_micros BIGINT NOT NULL DEFAULT 0,
    reserved_micros BIGINT NOT NULL DEFAULT 0,
    estimated BOOLEAN NOT NULL DEFAULT FALSE,
    upstream_request_id VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY api_usages_request_id_key (request_id),
    KEY api_usages_user_id_created_at (user_id, created_at),
    KEY api_usages_api_key_id_created_at (api_key_id, created_at),
    KEY api_usages_model_route_id_created_at (model_route_id, created_at),
    CONSTRAINT api_usages_users_api_usages
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT api_usages_api_keys_api_usages
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT api_usages_model_routes_api_usages
        FOREIGN KEY (model_route_id) REFERENCES model_routes (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
