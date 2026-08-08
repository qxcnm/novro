-- Novro migration 0019: allow one public model to use multiple provider routes.
ALTER TABLE model_routes
    DROP INDEX model_routes_public_name_key,
    ADD UNIQUE KEY modelroute_public_name_provider_id_upstream_model_id
        (public_name, provider_id, upstream_model_id);
