ALTER TABLE billing_groups
    ADD COLUMN kind ENUM('standard', 'composite') NOT NULL DEFAULT 'standard' AFTER code;

CREATE TABLE IF NOT EXISTS billing_group_compositions (
    id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    composite_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    member_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY billing_group_compositions_parent_member (composite_group_id, member_group_id),
    KEY billing_group_compositions_member_group (member_group_id),
    CONSTRAINT billing_group_compositions_parent
        FOREIGN KEY (composite_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION,
    CONSTRAINT billing_group_compositions_member
        FOREIGN KEY (member_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
