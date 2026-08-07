-- Novro migration 0012: preserve referenced configuration records after deletion.
ALTER TABLE providers
    ADD COLUMN deleted_at DATETIME(6) NULL AFTER updated_at,
    ADD KEY providers_deleted_at (deleted_at);

ALTER TABLE upstream_models
    ADD COLUMN deleted_at DATETIME(6) NULL AFTER updated_at,
    ADD KEY upstream_models_deleted_at (deleted_at);

ALTER TABLE model_routes
    ADD COLUMN deleted_at DATETIME(6) NULL AFTER updated_at,
    ADD KEY model_routes_deleted_at (deleted_at);

ALTER TABLE billing_groups
    ADD COLUMN deleted_at DATETIME(6) NULL AFTER updated_at,
    ADD KEY billing_groups_deleted_at (deleted_at);
