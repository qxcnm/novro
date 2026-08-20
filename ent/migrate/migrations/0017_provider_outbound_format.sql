ALTER TABLE providers
    ADD COLUMN outbound_format VARCHAR(32) NOT NULL DEFAULT '' AFTER protocols;
