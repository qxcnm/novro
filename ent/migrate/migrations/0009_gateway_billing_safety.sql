CREATE TABLE IF NOT EXISTS gateway_operations (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    idempotency_key_hash VARCHAR(64) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    endpoint ENUM('chat_completions', 'responses', 'messages') NOT NULL,
    status ENUM('processing', 'pending_settlement', 'pending_unknown', 'completed', 'failed') NOT NULL DEFAULT 'processing',
    reserved_micros BIGINT NOT NULL DEFAULT 0,
    settlement_json LONGTEXT NOT NULL,
    failure_code VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    api_key_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY gatewayoperation_api_key_id_idempotency_key_hash (api_key_id, idempotency_key_hash),
    KEY gatewayoperation_status_updated_at (status, updated_at),
    KEY gatewayoperation_user_id_created_at (user_id, created_at),
    CONSTRAINT gateway_operations_api_keys_gateway_operations
        FOREIGN KEY (api_key_id) REFERENCES api_keys (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT gateway_operations_users_gateway_operations
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

ALTER TABLE wallet_entries
    MODIFY COLUMN entry_type ENUM('manual_adjustment', 'top_up', 'referral_reward', 'usage_reservation', 'usage_refund', 'usage_settlement', 'usage_compensation') NOT NULL;
