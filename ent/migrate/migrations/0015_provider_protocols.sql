ALTER TABLE providers
    ADD COLUMN protocols JSON NULL AFTER protocol;

UPDATE providers
SET protocols = JSON_ARRAY(protocol)
WHERE protocols IS NULL;

ALTER TABLE providers
    MODIFY COLUMN protocols JSON NOT NULL;
