ALTER TABLE api_usages
    ADD INDEX apiusage_user_id_model_name_created_at (user_id, model_name, created_at),
    ADD INDEX apiusage_user_id_status_code_created_at (user_id, status_code, created_at);

ALTER TABLE wallet_entries
    ADD INDEX walletentry_wallet_id_entry_type_created_at (wallet_id, entry_type, created_at);
