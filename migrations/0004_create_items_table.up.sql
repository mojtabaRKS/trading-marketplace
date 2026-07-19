CREATE TABLE items (
    id             BIGSERIAL PRIMARY KEY,
    name           VARCHAR(160) NOT NULL,
    tier           VARCHAR(16)  NOT NULL, -- common|rare|legendary
    owner_guild_id BIGINT       NOT NULL,
    status         VARCHAR(16)  NOT NULL DEFAULT 'available',
    stock          INT          NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ,
    CONSTRAINT chk_item_tier        CHECK (tier IN ('common','rare','legendary')),
    CONSTRAINT chk_item_status      CHECK (status IN ('available','listed','in_auction','sold')),
    CONSTRAINT chk_item_stock_nonneg CHECK (stock >= 0)
);

CREATE INDEX idx_items_tier  ON items (tier);
CREATE INDEX idx_items_owner ON items (owner_guild_id);
