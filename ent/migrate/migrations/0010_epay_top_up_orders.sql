-- Novro migration 0010: EPay top-up orders and wallet credits.
ALTER TABLE wallet_entries
    MODIFY COLUMN entry_type ENUM('manual_adjustment', 'top_up', 'usage_reservation', 'usage_refund', 'usage_settlement') NOT NULL;

CREATE TABLE IF NOT EXISTS top_up_orders (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    out_trade_no VARCHAR(64) NOT NULL,
    provider ENUM('epay') NOT NULL DEFAULT 'epay',
    channel VARCHAR(32) NOT NULL,
    amount_micros BIGINT NOT NULL,
    status ENUM('pending', 'paid') NOT NULL DEFAULT 'pending',
    provider_trade_no VARCHAR(128) NULL,
    paid_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY top_up_orders_out_trade_no_key (out_trade_no),
    UNIQUE KEY top_up_orders_provider_trade_no_key (provider_trade_no),
    KEY top_up_orders_user_id_created_at (user_id, created_at),
    KEY top_up_orders_status_created_at (status, created_at),
    CONSTRAINT top_up_orders_users_top_up_orders
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT top_up_orders_amount_positive CHECK (amount_micros > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
