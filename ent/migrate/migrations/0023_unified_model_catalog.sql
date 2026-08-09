-- Novro migration 0023: maintain one global model and price card per exact model ID.
-- Provider synchronization discovers IDs; provider routes share the catalog record.

ALTER TABLE model_routes
    MODIFY COLUMN public_name VARCHAR(256) NOT NULL;

ALTER TABLE api_usages
    MODIFY COLUMN model_name VARCHAR(256) NOT NULL DEFAULT '';

CREATE TEMPORARY TABLE novro_model_canonical_map (
    model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    canonical_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    KEY novro_model_canonical_map_canonical_id (canonical_id)
) ENGINE=InnoDB;

INSERT INTO novro_model_canonical_map (model_id, canonical_id)
SELECT ranked.id, ranked.canonical_id
FROM (
    SELECT
        id,
        FIRST_VALUE(id) OVER (
            PARTITION BY LOWER(upstream_name)
            ORDER BY
                CASE WHEN id IN (
                    '10000000-0000-0000-0000-000000000001',
                    '10000000-0000-0000-0000-000000000002',
                    '20000000-0000-0000-0000-000000000001',
                    '20000000-0000-0000-0000-000000000002',
                    '30000000-0000-0000-0000-000000000001',
                    '30000000-0000-0000-0000-000000000002',
                    '30000000-0000-0000-0000-000000000003',
                    '30000000-0000-0000-0000-000000000004'
                ) THEN 0 ELSE 1 END,
                (deleted_at IS NULL) DESC,
                pricing_configured DESC,
                (status = 'active') DESC,
                created_at ASC,
                id ASC
        ) AS canonical_id
    FROM upstream_models
) AS ranked;

CREATE TEMPORARY TABLE novro_model_canonical_state (
    canonical_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    has_visible BOOLEAN NOT NULL,
    has_active BOOLEAN NOT NULL,
    latest_updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB;

INSERT INTO novro_model_canonical_state (canonical_id, has_visible, has_active, latest_updated_at)
SELECT
    model_map.canonical_id,
    MAX(model.deleted_at IS NULL),
    MAX(model.deleted_at IS NULL AND model.status = 'active'),
    MAX(model.updated_at)
FROM novro_model_canonical_map AS model_map
JOIN upstream_models AS model ON model.id = model_map.model_id
GROUP BY model_map.canonical_id;

UPDATE upstream_models AS canonical_model
JOIN novro_model_canonical_state AS canonical_state ON canonical_state.canonical_id = canonical_model.id
SET canonical_model.deleted_at = IF(canonical_state.has_visible, NULL, canonical_model.deleted_at),
    canonical_model.status = IF(
        canonical_model.pricing_configured AND canonical_state.has_active,
        'active',
        'disabled'
    ),
    canonical_model.updated_at = GREATEST(canonical_model.updated_at, canonical_state.latest_updated_at);

CREATE TEMPORARY TABLE novro_route_targets (
    route_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    canonical_model_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    target_public_name VARCHAR(256) NOT NULL,
    KEY novro_route_targets_model (canonical_model_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO novro_route_targets (route_id, canonical_model_id, target_public_name)
SELECT
    route.id,
    model_map.canonical_id,
    CASE
        WHEN LOWER(route.public_name) = LOWER(CONCAT(configured_provider.code, '-', route.upstream_name))
            THEN canonical_model.upstream_name
        ELSE route.public_name
    END
FROM model_routes AS route
JOIN providers AS configured_provider ON configured_provider.id = route.provider_id
JOIN novro_model_canonical_map AS model_map ON model_map.model_id = route.upstream_model_id
JOIN upstream_models AS canonical_model ON canonical_model.id = model_map.canonical_id;

CREATE TEMPORARY TABLE novro_route_canonical_map (
    route_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL PRIMARY KEY,
    canonical_route_id CHAR(36) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    KEY novro_route_canonical_map_canonical_id (canonical_route_id)
) ENGINE=InnoDB;

INSERT INTO novro_route_canonical_map (route_id, canonical_route_id)
SELECT ranked.route_id, ranked.canonical_route_id
FROM (
    SELECT
        route.id AS route_id,
        FIRST_VALUE(route.id) OVER (
            PARTITION BY targets.target_public_name, route.provider_id, targets.canonical_model_id
            ORDER BY
                (route.deleted_at IS NULL) DESC,
                (route.status = 'active') DESC,
                route.created_at ASC,
                route.id ASC
        ) AS canonical_route_id
    FROM model_routes AS route
    JOIN novro_route_targets AS targets ON targets.route_id = route.id
) AS ranked;

UPDATE api_usages AS usage_record
JOIN novro_route_canonical_map AS route_map ON route_map.route_id = usage_record.model_route_id
SET usage_record.model_route_id = route_map.canonical_route_id
WHERE route_map.route_id <> route_map.canonical_route_id;

DELETE duplicate_route
FROM model_routes AS duplicate_route
JOIN novro_route_canonical_map AS route_map ON route_map.route_id = duplicate_route.id
WHERE route_map.route_id <> route_map.canonical_route_id;

UPDATE model_routes AS route
JOIN novro_route_targets AS targets ON targets.route_id = route.id
JOIN novro_route_canonical_map AS route_map
  ON route_map.route_id = route.id
 AND route_map.canonical_route_id = route.id
JOIN upstream_models AS canonical_model ON canonical_model.id = targets.canonical_model_id
SET route.upstream_model_id = targets.canonical_model_id,
    route.public_name = targets.target_public_name,
    route.upstream_name = canonical_model.upstream_name,
    route.display_name = canonical_model.display_name,
    route.input_price_micros = canonical_model.input_price_micros,
    route.output_price_micros = canonical_model.output_price_micros,
    route.status = IF(
        route.deleted_at IS NULL
        AND canonical_model.deleted_at IS NULL
        AND canonical_model.status = 'active'
        AND canonical_model.pricing_configured,
        route.status,
        'disabled'
    ),
    route.updated_at = GREATEST(route.updated_at, canonical_model.updated_at);

UPDATE api_usages AS usage_record
JOIN novro_model_canonical_map AS model_map ON model_map.model_id = usage_record.upstream_model_id
SET usage_record.upstream_model_id = model_map.canonical_id
WHERE model_map.model_id <> model_map.canonical_id;

DELETE duplicate_model
FROM upstream_models AS duplicate_model
JOIN novro_model_canonical_map AS model_map ON model_map.model_id = duplicate_model.id
WHERE model_map.model_id <> model_map.canonical_id;

ALTER TABLE upstream_models
    DROP INDEX upstream_models_provider_name_upstream_name,
    ADD UNIQUE KEY upstream_models_upstream_name_key (upstream_name);

DROP TEMPORARY TABLE novro_route_canonical_map;
DROP TEMPORARY TABLE novro_route_targets;
DROP TEMPORARY TABLE novro_model_canonical_state;
DROP TEMPORARY TABLE novro_model_canonical_map;
