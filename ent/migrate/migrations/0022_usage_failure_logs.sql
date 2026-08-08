-- Novro migration 0022: retain failed gateway requests in usage logs.
ALTER TABLE api_usages
    ADD COLUMN status_code INT NOT NULL DEFAULT 200 AFTER endpoint,
    ADD COLUMN error_code VARCHAR(64) NOT NULL DEFAULT '' AFTER status_code,
    ADD COLUMN error_message VARCHAR(1024) NOT NULL DEFAULT '' AFTER error_code,
    ADD COLUMN duration_ms BIGINT NOT NULL DEFAULT 0 AFTER finished_at;
