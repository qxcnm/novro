ALTER TABLE model_routes
    ADD COLUMN billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER deleted_at;

UPDATE model_routes AS routes
JOIN providers AS providers ON providers.id = routes.provider_id
SET routes.billing_group_id = providers.billing_group_id
WHERE routes.billing_group_id IS NULL;

ALTER TABLE model_routes
    MODIFY COLUMN billing_group_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    DROP INDEX modelroute_public_name_provider_id_upstream_model_id,
    ADD UNIQUE KEY modelroute_group_public_provider_model
        (billing_group_id, public_name, provider_id, upstream_model_id),
    ADD KEY modelroute_billing_group_id_status (billing_group_id, status),
    ADD CONSTRAINT model_routes_billing_groups_model_routes
        FOREIGN KEY (billing_group_id) REFERENCES billing_groups (id)
        ON DELETE NO ACTION ON UPDATE NO ACTION;

ALTER TABLE providers
    DROP FOREIGN KEY providers_billing_groups_providers,
    DROP INDEX provider_billing_group_id_status,
    DROP COLUMN billing_group_id;
