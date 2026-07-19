CREATE TABLE auctions (
    id              BIGSERIAL PRIMARY KEY,
    item_id         BIGINT      NOT NULL,
    seller_guild_id BIGINT      NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'active',
    starts_at       TIMESTAMPTZ,
    ends_at         TIMESTAMPTZ,
    highest_bid_id  BIGINT,
    winner_guild_id BIGINT,
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ,
    CONSTRAINT chk_auction_status CHECK (status IN ('active','settled','cancelled'))
);

CREATE INDEX idx_auctions_item    ON auctions (item_id);
CREATE INDEX idx_auctions_seller  ON auctions (seller_guild_id);
CREATE INDEX idx_auctions_ends_at ON auctions (ends_at);

-- Business rule: at most one active auction per item.
CREATE UNIQUE INDEX uniq_active_auction_per_item ON auctions (item_id) WHERE status = 'active';
