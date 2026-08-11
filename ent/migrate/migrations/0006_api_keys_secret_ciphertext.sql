ALTER TABLE api_keys
    ADD COLUMN key_secret_ciphertext VARCHAR(256) NOT NULL DEFAULT '' AFTER key_hash;
