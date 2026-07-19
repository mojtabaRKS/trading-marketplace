CREATE TABLE listings (
    id              BIGSERIAL PRIMARY KEY,
    item_id         BIGINT      NOT NULL,
    seller_guild_id BIGINT      NOT NULL,
    price           BIGINT      NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'open',
    buyer_guild_id  BIGINT,
    created_at      TIMESTAMPTZ,
    sold_at         TIMESTAMPTZ,
    CONSTRAINT chk_listing_price_pos CHECK (price > 0),
    CONSTRAINT chk_listing_status    CHECK (status IN ('open','sold','cancelled'))
);

CREATE INDEX idx_listings_item   ON listings (item_id);
CREATE INDEX idx_listings_seller ON listings (seller_guild_id);
