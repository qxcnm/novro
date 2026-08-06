-- Novro migration 0006: make usage reservation and refund retries idempotent.
SET @novro_wallet_entry_index_exists = (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'wallet_entries'
      AND index_name = 'wallet_entries_wallet_id_reference_id_entry_type'
);

SET @novro_wallet_entry_index_sql = IF(
    @novro_wallet_entry_index_exists = 0,
    'CREATE UNIQUE INDEX wallet_entries_wallet_id_reference_id_entry_type ON wallet_entries (wallet_id, reference_id, entry_type)',
    'SELECT 1'
);

PREPARE novro_wallet_entry_index_statement FROM @novro_wallet_entry_index_sql;
EXECUTE novro_wallet_entry_index_statement;
DEALLOCATE PREPARE novro_wallet_entry_index_statement;
