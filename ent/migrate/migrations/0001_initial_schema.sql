-- Novro migration 0001: initial schema for fresh deployments.
-- This migration is intentionally squashed for empty-database redeploys.

CREATE TABLE IF NOT EXISTS billing_groups (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    code VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    multiplier_bps BIGINT NOT NULL DEFAULT 10000,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY billing_groups_code_key (code),
    KEY billinggroup_status_created_at (status, created_at),
    KEY billinggroup_is_default (is_default),
    KEY billinggroup_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO billing_groups (id, code, display_name, multiplier_bps, is_default, status, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000001', 'default', '默认分组', 10000, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE updated_at = updated_at;

CREATE TABLE IF NOT EXISTS users (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    invite_code VARCHAR(16) NOT NULL,
    username VARCHAR(64) NOT NULL,
    email VARCHAR(320) NULL,
    display_name VARCHAR(128) NOT NULL DEFAULT '',
    password_hash VARCHAR(255) NULL,
    is_system_admin BOOLEAN NOT NULL DEFAULT FALSE,
    role ENUM('admin', 'member') NOT NULL DEFAULT 'member',
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    last_login_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    referred_by_user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    PRIMARY KEY (id),
    UNIQUE KEY users_invite_code_key (invite_code),
    UNIQUE KEY users_username_key (username),
    UNIQUE KEY users_email_key (email),
    KEY user_role_status (role, status),
    KEY user_referred_by_user_id_created_at (referred_by_user_id, created_at),
    CONSTRAINT users_users_referrals
        FOREIGN KEY (referred_by_user_id) REFERENCES users (id)
        ON DELETE SET NULL ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS system_settings (
    `key` VARCHAR(128) NOT NULL,
    `value` VARCHAR(1024) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO system_settings (`key`, `value`, created_at, updated_at)
VALUES ('referral_reward_bps', '1000', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))
ON DUPLICATE KEY UPDATE updated_at = updated_at;

CREATE TABLE IF NOT EXISTS payment_configs (
    provider VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    api_url VARCHAR(512) NOT NULL DEFAULT '',
    merchant_id VARCHAR(128) NOT NULL DEFAULT '',
    encrypted_merchant_key VARCHAR(2048) NOT NULL DEFAULT '',
    site_name VARCHAR(64) NOT NULL DEFAULT 'Novro',
    channels VARCHAR(512) NOT NULL DEFAULT '',
    methods_json TEXT NOT NULL,
    min_top_up_micros BIGINT NOT NULL DEFAULT 10000,
    max_top_up_micros BIGINT NOT NULL DEFAULT 50000000000,
    preset_amounts_json TEXT NOT NULL,
    bonus_tiers_json TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (provider)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS email_smtp_configs (
    id VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    host VARCHAR(255) NOT NULL DEFAULT '',
    port INT NOT NULL DEFAULT 587,
    username VARCHAR(320) NOT NULL DEFAULT '',
    encrypted_password VARCHAR(2048) NOT NULL DEFAULT '',
    from_address VARCHAR(320) NOT NULL DEFAULT '',
    security VARCHAR(16) NOT NULL DEFAULT 'starttls',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS email_verification_codes (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    email VARCHAR(320) NOT NULL,
    code_hash VARCHAR(64) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    consumed_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY emailverificationcode_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS providers (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    code VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    protocol ENUM('openai', 'anthropic') NOT NULL,
    base_url VARCHAR(512) NOT NULL,
    model_list_path VARCHAR(512) NOT NULL DEFAULT '',
    encrypted_api_key VARCHAR(2048) NOT NULL,
    api_key_hint VARCHAR(8) NOT NULL,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted_at DATETIME(6) NULL,
    billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY providers_code_key (code),
    KEY provider_billing_group_id_status (billing_group_id, status),
    KEY provider_status_created_at (status, created_at),
    KEY provider_deleted_at (deleted_at),
    CONSTRAINT providers_billing_groups_providers
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_sessions (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    last_seen_at DATETIME(6) NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY user_sessions_token_hash_key (token_hash),
    KEY user_sessions_user_id (user_id),
    CONSTRAINT user_sessions_users_sessions
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_identities (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    issuer VARCHAR(512) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    email VARCHAR(320) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY useridentity_issuer_subject (issuer, subject),
    UNIQUE KEY useridentity_user_id_issuer (user_id, issuer),
    CONSTRAINT user_identities_users_identities
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS wallets (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    balance_micros BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY wallets_user_id_key (user_id),
    CONSTRAINT wallets_users_wallet
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS top_up_orders (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    out_trade_no VARCHAR(64) NOT NULL,
    provider ENUM('epay') NOT NULL DEFAULT 'epay',
    channel VARCHAR(32) NOT NULL,
    amount_micros BIGINT NOT NULL,
    credited_micros BIGINT NOT NULL,
    status ENUM('pending', 'paid') NOT NULL DEFAULT 'pending',
    provider_trade_no VARCHAR(128) NULL,
    paid_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY top_up_orders_out_trade_no_key (out_trade_no),
    UNIQUE KEY top_up_orders_provider_trade_no_key (provider_trade_no),
    KEY topuporder_user_id_created_at (user_id, created_at),
    KEY topuporder_status_created_at (status, created_at),
    CONSTRAINT top_up_orders_users_top_up_orders
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS api_keys (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    name VARCHAR(64) NOT NULL,
    key_prefix VARCHAR(16) NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    status ENUM('active', 'revoked') NOT NULL DEFAULT 'active',
    last_used_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    revoked_at DATETIME(6) NULL,
    billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY api_keys_key_hash_key (key_hash),
    KEY apikey_user_id_status (user_id, status),
    KEY apikey_billing_group_id_status (billing_group_id, status),
    KEY apikey_status_created_at (status, created_at),
    CONSTRAINT api_keys_billing_groups_api_keys
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT api_keys_users_api_keys
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS upstream_models (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    provider_name VARCHAR(128) NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    input_price_micros BIGINT NOT NULL DEFAULT 0,
    output_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_read_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0,
    request_price_micros BIGINT NOT NULL DEFAULT 0,
    pricing_configured BOOLEAN NOT NULL DEFAULT TRUE,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted_at DATETIME(6) NULL,
    PRIMARY KEY (id),
    UNIQUE KEY upstreammodel_upstream_name (upstream_name),
    KEY upstreammodel_provider_name_status (provider_name, status),
    KEY upstreammodel_status_created_at (status, created_at),
    KEY upstreammodel_deleted_at (deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS model_routes (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    public_name VARCHAR(256) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    upstream_name VARCHAR(256) NOT NULL,
    input_price_micros BIGINT NOT NULL,
    output_price_micros BIGINT NOT NULL,
    status ENUM('active', 'disabled') NOT NULL DEFAULT 'active',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted_at DATETIME(6) NULL,
    provider_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    upstream_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    PRIMARY KEY (id),
    UNIQUE KEY modelroute_public_name_provider_id_upstream_model_id (public_name, provider_id, upstream_model_id),
    KEY modelroute_provider_id_status (provider_id, status),
    KEY modelroute_upstream_model_id_status (upstream_model_id, status),
    KEY modelroute_status_created_at (status, created_at),
    KEY modelroute_deleted_at (deleted_at),
    CONSTRAINT model_routes_providers_model_routes
        FOREIGN KEY (provider_id) REFERENCES providers (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT model_routes_upstream_models_model_routes
        FOREIGN KEY (upstream_model_id) REFERENCES upstream_models (id)
        ON DELETE SET NULL ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS wallet_entries (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    reference_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    entry_type ENUM('manual_adjustment', 'top_up', 'referral_reward', 'usage_reservation', 'usage_refund', 'usage_settlement') NOT NULL,
    amount_micros BIGINT NOT NULL,
    balance_after_micros BIGINT NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    actor_user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    wallet_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    KEY walletentry_wallet_id_created_at (wallet_id, created_at),
    KEY walletentry_reference_id (reference_id),
    UNIQUE KEY walletentry_wallet_id_reference_id_entry_type (wallet_id, reference_id, entry_type),
    CONSTRAINT wallet_entries_users_wallet_entries
        FOREIGN KEY (actor_user_id) REFERENCES users (id)
        ON DELETE SET NULL ON UPDATE NO ACTION,
    CONSTRAINT wallet_entries_wallets_entries
        FOREIGN KEY (wallet_id) REFERENCES wallets (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS api_usages (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    request_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    endpoint ENUM('chat_completions', 'responses', 'messages') NOT NULL,
    status_code INT NOT NULL DEFAULT 200,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    error_message VARCHAR(1024) NOT NULL DEFAULT '',
    input_tokens INT NOT NULL DEFAULT 0,
    uncached_input_tokens INT NOT NULL DEFAULT 0,
    cache_read_input_tokens INT NOT NULL DEFAULT 0,
    cache_write_input_tokens INT NOT NULL DEFAULT 0,
    cache_write_1h_input_tokens INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    input_price_micros BIGINT NOT NULL DEFAULT 0,
    output_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_read_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_price_micros BIGINT NOT NULL DEFAULT 0,
    cache_write_1h_price_micros BIGINT NOT NULL DEFAULT 0,
    request_price_micros BIGINT NOT NULL DEFAULT 0,
    base_cost_micros BIGINT NOT NULL DEFAULT 0,
    multiplier_bps BIGINT NOT NULL DEFAULT 10000,
    cost_micros BIGINT NOT NULL DEFAULT 0,
    reserved_micros BIGINT NOT NULL DEFAULT 0,
    estimated BOOLEAN NOT NULL DEFAULT FALSE,
    upstream_request_id VARCHAR(255) NOT NULL DEFAULT '',
    model_name VARCHAR(256) NOT NULL DEFAULT '',
    upstream_model_name VARCHAR(256) NOT NULL DEFAULT '',
    billing_group_code VARCHAR(64) NOT NULL DEFAULT '',
    billing_group_name VARCHAR(128) NOT NULL DEFAULT '',
    calculation_version VARCHAR(32) NOT NULL DEFAULT 'v1',
    created_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NOT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    api_key_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    model_route_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    upstream_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY api_usages_request_id_key (request_id),
    KEY apiusage_user_id_created_at (user_id, created_at),
    KEY apiusage_api_key_id_created_at (api_key_id, created_at),
    KEY apiusage_model_route_id_created_at (model_route_id, created_at),
    KEY apiusage_upstream_model_id_created_at (upstream_model_id, created_at),
    KEY apiusage_billing_group_id_created_at (billing_group_id, created_at),
    CONSTRAINT api_usages_api_keys_api_usages
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT api_usages_billing_groups_api_usages
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE SET NULL ON UPDATE NO ACTION,
    CONSTRAINT api_usages_model_routes_api_usages
        FOREIGN KEY (model_route_id) REFERENCES model_routes (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT api_usages_upstream_models_api_usages
        FOREIGN KEY (upstream_model_id) REFERENCES upstream_models (id)
        ON DELETE SET NULL ON UPDATE NO ACTION,
    CONSTRAINT api_usages_users_api_usages
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
