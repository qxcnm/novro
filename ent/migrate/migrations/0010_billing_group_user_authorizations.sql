CREATE TABLE IF NOT EXISTS billing_group_authorized_users (
    billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    user_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (billing_group_id, user_id),
    KEY billing_group_authorized_users_user_id (user_id),
    CONSTRAINT billing_group_authorized_users_billing_group
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE CASCADE ON UPDATE NO ACTION,
    CONSTRAINT billing_group_authorized_users_user
        FOREIGN KEY (user_id) REFERENCES users (id)
        ON DELETE CASCADE ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Preserve the effective access granted by migration 0008 while moving from a
-- global user flag to per-group authorization. The legacy column remains in
-- place for forward-only production upgrades, but application code no longer
-- reads or writes it.
INSERT IGNORE INTO billing_group_authorized_users (billing_group_id, user_id)
SELECT billing_groups.id, users.id
FROM billing_groups
JOIN users ON users.can_access_hidden_groups = TRUE
WHERE billing_groups.is_hidden = TRUE
  AND billing_groups.deleted_at IS NULL
  AND users.role = 'member';
