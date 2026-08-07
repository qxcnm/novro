-- Novro migration 0013: unify local and OIDC account email identity.
ALTER TABLE users
    ADD COLUMN email VARCHAR(320) NULL AFTER username;

-- Backfill one unambiguous normalized OIDC email per existing user. If the
-- same email belongs to more than one user, leave it unset for an
-- administrator to resolve instead of merging identities by email.
UPDATE users
JOIN (
    SELECT user_id, normalized_email
    FROM (
        SELECT user_id,
               normalized_email,
               ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY normalized_email) AS user_email_rank,
               COUNT(*) OVER (PARTITION BY normalized_email) AS email_user_count
        FROM (
            SELECT user_id, LOWER(TRIM(email)) AS normalized_email
            FROM user_identities
            WHERE TRIM(email) <> ''
            GROUP BY user_id, LOWER(TRIM(email))
        ) AS distinct_identity_emails
    ) AS ranked_identity_emails
    WHERE user_email_rank = 1 AND email_user_count = 1
) AS identity_emails ON identity_emails.user_id = users.id
SET users.email = identity_emails.normalized_email
WHERE users.email IS NULL;

ALTER TABLE users
    ADD UNIQUE KEY users_email (email);
