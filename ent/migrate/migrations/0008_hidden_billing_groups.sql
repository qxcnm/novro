ALTER TABLE billing_groups
    ADD COLUMN is_hidden BOOLEAN NOT NULL DEFAULT FALSE AFTER is_default;

ALTER TABLE users
    ADD COLUMN can_access_hidden_groups BOOLEAN NOT NULL DEFAULT FALSE AFTER is_system_admin;
