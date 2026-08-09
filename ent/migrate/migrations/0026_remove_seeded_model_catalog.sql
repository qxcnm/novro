-- Novro migration 0026: remove the historical built-in model catalog.
-- Models become visible again only when an upstream provider advertises them
-- and an administrator selects the synced result. Keep rows and references
-- for billing history, then let providersync restore an advertised model.

UPDATE model_routes
SET status = 'disabled',
    deleted_at = COALESCE(deleted_at, UTC_TIMESTAMP(6)),
    updated_at = UTC_TIMESTAMP(6)
WHERE upstream_model_id IN (
    '10000000-0000-0000-0000-000000000001',
    '10000000-0000-0000-0000-000000000002',
    '20000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000002',
    '30000000-0000-0000-0000-000000000001',
    '30000000-0000-0000-0000-000000000002',
    '30000000-0000-0000-0000-000000000003',
    '30000000-0000-0000-0000-000000000004'
);

UPDATE upstream_models
SET status = 'disabled',
    deleted_at = COALESCE(deleted_at, UTC_TIMESTAMP(6)),
    updated_at = UTC_TIMESTAMP(6)
WHERE id IN (
    '10000000-0000-0000-0000-000000000001',
    '10000000-0000-0000-0000-000000000002',
    '20000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000002',
    '30000000-0000-0000-0000-000000000001',
    '30000000-0000-0000-0000-000000000002',
    '30000000-0000-0000-0000-000000000003',
    '30000000-0000-0000-0000-000000000004'
);
