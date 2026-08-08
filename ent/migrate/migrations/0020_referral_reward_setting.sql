-- Novro migration 0020: persist the administrator-managed referral reward rate.
INSERT IGNORE INTO system_settings (`key`, `value`, created_at, updated_at)
VALUES ('referral_reward_bps', '1000', CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6));
