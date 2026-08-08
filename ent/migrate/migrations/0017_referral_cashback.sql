-- Novro migration 0017: referral links and cashback wallet entries.
ALTER TABLE users
    ADD COLUMN invite_code VARCHAR(16) NULL AFTER billing_group_id,
    ADD COLUMN referred_by_user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER invite_code;

UPDATE users
SET invite_code = UPPER(LEFT(REPLACE(id, '-', ''), 12))
WHERE invite_code IS NULL OR invite_code = '';

ALTER TABLE users
    MODIFY COLUMN invite_code VARCHAR(16) NOT NULL,
    ADD UNIQUE KEY users_invite_code_key (invite_code),
    ADD KEY user_referred_by_user_id_created_at (referred_by_user_id, created_at),
    ADD CONSTRAINT users_users_referrals
        FOREIGN KEY (referred_by_user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION;

ALTER TABLE wallet_entries
    MODIFY COLUMN entry_type ENUM('manual_adjustment', 'top_up', 'referral_reward', 'usage_reservation', 'usage_refund', 'usage_settlement') NOT NULL;
