-- Novro migration 0002: external identities and atomic first-run setup.
ALTER TABLE users
    MODIFY COLUMN password_hash VARCHAR(255) NULL;

CREATE TABLE IF NOT EXISTS user_identities (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    issuer VARCHAR(512) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    email VARCHAR(320) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY user_identities_issuer_subject (issuer, subject),
    UNIQUE KEY user_identities_user_issuer (user_id, issuer),
    CONSTRAINT user_identities_users_identities
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS system_settings (
    `key` VARCHAR(128) NOT NULL,
    `value` VARCHAR(1024) NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Existing installations created before this migration already have an
-- administrator. Mark them initialized so setup can never reopen later.
INSERT IGNORE INTO system_settings (`key`, `value`, created_at, updated_at)
SELECT 'initial_admin_created', 'complete', CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6)
WHERE EXISTS (SELECT 1 FROM users LIMIT 1);
