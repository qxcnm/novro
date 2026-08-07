-- Novro migration 0014: flexible payment methods, recharge rules, and credited amounts.
ALTER TABLE payment_configs
    ADD COLUMN methods_json TEXT NOT NULL AFTER channels,
    ADD COLUMN min_top_up_micros BIGINT NOT NULL DEFAULT 1000000 AFTER methods_json,
    ADD COLUMN max_top_up_micros BIGINT NOT NULL DEFAULT 50000000000 AFTER min_top_up_micros,
    ADD COLUMN preset_amounts_json TEXT NOT NULL AFTER max_top_up_micros,
    ADD COLUMN bonus_tiers_json TEXT NOT NULL AFTER preset_amounts_json;

UPDATE payment_configs
SET preset_amounts_json = '[10000000,50000000,100000000,500000000]',
    methods_json = '[]',
    bonus_tiers_json = '[]'
WHERE preset_amounts_json = '' OR methods_json = '' OR bonus_tiers_json = '';

ALTER TABLE top_up_orders
    ADD COLUMN credited_micros BIGINT NOT NULL DEFAULT 0 AFTER amount_micros;

UPDATE top_up_orders
SET credited_micros = amount_micros
WHERE credited_micros = 0;

ALTER TABLE top_up_orders
    MODIFY COLUMN credited_micros BIGINT NOT NULL,
    ADD CONSTRAINT top_up_orders_credited_positive CHECK (credited_micros > 0);
