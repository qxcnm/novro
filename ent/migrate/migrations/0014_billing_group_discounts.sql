ALTER TABLE billing_groups
    ADD COLUMN discount_name VARCHAR(64) NOT NULL DEFAULT '' AFTER multiplier_bps,
    ADD COLUMN discount_multiplier_bps BIGINT NOT NULL DEFAULT 10000 AFTER discount_name,
    ADD COLUMN discount_starts_at DATETIME(6) NULL AFTER discount_multiplier_bps,
    ADD COLUMN discount_ends_at DATETIME(6) NULL AFTER discount_starts_at,
    ADD KEY billinggroup_discount_window (discount_starts_at, discount_ends_at);
