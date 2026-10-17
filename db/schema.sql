CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email            TEXT NOT NULL,
    name             TEXT,
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    accountant_email TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_email_lower_chk     CHECK (email = lower(btrim(email))),
    CONSTRAINT users_provider_chk        CHECK (provider IN ('google', 'apple')),
    CONSTRAINT users_email_uniq          UNIQUE (email),
    CONSTRAINT users_provider_id_uniq    UNIQUE (provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS users_accountant_email_idx
    ON users (lower(accountant_email))
    WHERE accountant_email IS NOT NULL;

-- Fiş görselleri sunucuda saklanmaz; yalnızca metadata ve cihaz içi yol yazılır.
CREATE TABLE IF NOT EXISTS receipts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    store_name       TEXT NOT NULL,
    total_amount     NUMERIC(12, 2) NOT NULL,
    currency         VARCHAR(3) NOT NULL,
    category         TEXT NOT NULL,
    receipt_date     TIMESTAMPTZ NOT NULL,
    local_image_path TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT receipts_store_name_chk CHECK (char_length(btrim(store_name)) BETWEEN 1 AND 200),
    CONSTRAINT receipts_amount_chk     CHECK (total_amount >= 0),
    CONSTRAINT receipts_currency_chk   CHECK (currency = upper(currency) AND char_length(currency) = 3),
    CONSTRAINT receipts_category_chk   CHECK (category IN (
        'fuel', 'food', 'groceries', 'repair', 'software', 'office',
        'travel', 'health', 'entertainment', 'clothing', 'utilities', 'other'
    )),
    CONSTRAINT receipts_date_chk       CHECK (receipt_date <= now() + INTERVAL '1 day')
);

CREATE INDEX IF NOT EXISTS receipts_user_created_idx ON receipts (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS receipts_user_date_idx    ON receipts (user_id, receipt_date DESC);
CREATE INDEX IF NOT EXISTS receipts_user_category_idx ON receipts (user_id, category);