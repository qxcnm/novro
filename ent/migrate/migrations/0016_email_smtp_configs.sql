-- Novro migration 0016: administrator-managed SMTP configuration.
CREATE TABLE IF NOT EXISTS email_smtp_configs (
    id VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    host VARCHAR(255) NOT NULL DEFAULT '',
    port INT NOT NULL DEFAULT 587,
    username VARCHAR(320) NOT NULL DEFAULT '',
    encrypted_password VARCHAR(2048) NOT NULL DEFAULT '',
    from_address VARCHAR(320) NOT NULL DEFAULT '',
    security VARCHAR(16) NOT NULL DEFAULT 'starttls',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT email_smtp_configs_port_valid CHECK (port BETWEEN 1 AND 65535),
    CONSTRAINT email_smtp_configs_security_valid CHECK (security IN ('none', 'starttls', 'ssl'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
