CREATE TABLE IF NOT EXISTS users (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email            TEXT NOT NULL,
    name             TEXT,
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    accountant_email TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lower_chk  CHECK (email = lower(btrim(email))),
    CONSTRAINT users_provider_chk     CHECK (provider IN ('google', 'apple')),
    CONSTRAINT users_email_uniq       UNIQUE (email),
    CONSTRAINT users_provider_id_uniq UNIQUE (provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS users_accountant_email_idx
    ON users (lower(accountant_email))
    WHERE accountant_email IS NOT NULL;