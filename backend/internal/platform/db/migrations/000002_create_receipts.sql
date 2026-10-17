-- Fiş kayıtları.
--
-- Tasarım notu: fiş görselleri sunucuda saklanmaz. Mobil cihaz görseli yerel
-- dosya sisteminde tutar, buraya yalnızca metinsel metadata ve cihaz içi yol
-- (local_image_path) yazılır. Bu nedenle local_image_path tek başına bir
-- dosya referansı değil, o cihazda çözümlenecek bir işaretçidir.

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

    CONSTRAINT receipts_store_name_chk   CHECK (char_length(btrim(store_name)) BETWEEN 1 AND 200),
    CONSTRAINT receipts_amount_chk       CHECK (total_amount >= 0),
    CONSTRAINT receipts_currency_chk     CHECK (currency = upper(currency) AND char_length(currency) = 3),
    CONSTRAINT receipts_category_chk     CHECK (category IN (
        'fuel', 'food', 'groceries', 'repair', 'software', 'office',
        'travel', 'health', 'entertainment', 'clothing', 'utilities', 'other'
    )),
    -- Fiş tarihi geçmişte olmalı; 1 günlük saat farkı toleransı cihaz saat kaymasına karşı.
    CONSTRAINT receipts_date_chk         CHECK (receipt_date <= now() + INTERVAL '1 day')
);

-- Kullanıcının en yeni fişleri (varsayılan liste görünümü).
CREATE INDEX IF NOT EXISTS receipts_user_created_idx
    ON receipts (user_id, created_at DESC);

-- Aylık kategori dökümü / dönemsel raporlar.
CREATE INDEX IF NOT EXISTS receipts_user_date_idx
    ON receipts (user_id, receipt_date DESC);

CREATE INDEX IF NOT EXISTS receipts_user_category_idx
    ON receipts (user_id, category);
