-- Novro migration 0018: allow one-cent top-ups for low-cost payment testing.
ALTER TABLE payment_configs
    MODIFY COLUMN min_top_up_micros BIGINT NOT NULL DEFAULT 10000;

WITH RECURSIVE migrated_payment_methods (provider, methods_json, method_index, method_count) AS (
    SELECT provider, methods_json, 0, JSON_LENGTH(methods_json)
    FROM payment_configs
    WHERE min_top_up_micros = 1000000
      AND JSON_VALID(methods_json)
    UNION ALL
    SELECT provider,
           CASE
               WHEN JSON_UNQUOTE(JSON_EXTRACT(methods_json, CONCAT('$[', method_index, '].min_micros'))) = '1000000'
                   THEN JSON_SET(methods_json, CONCAT('$[', method_index, '].min_micros'), 10000)
               ELSE methods_json
           END,
           method_index + 1,
           method_count
    FROM migrated_payment_methods
    WHERE method_index < method_count
)
UPDATE payment_configs AS payment_config
JOIN migrated_payment_methods AS migrated
  ON migrated.provider = payment_config.provider
 AND migrated.method_index = migrated.method_count
SET payment_config.methods_json = migrated.methods_json;

UPDATE payment_configs
SET min_top_up_micros = 10000
WHERE min_top_up_micros = 1000000;
