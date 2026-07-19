CREATE TABLE bids (
    id              BIGSERIAL PRIMARY KEY,
    auction_id      BIGINT      NOT NULL,
    bidder_guild_id BIGINT      NOT NULL,
    amount          BIGINT      NOT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ,
    CONSTRAINT chk_bid_amount_pos CHECK (amount > 0),
    CONSTRAINT chk_bid_status     CHECK (status IN ('active','released','won'))
);

CREATE INDEX idx_bids_auction ON bids (auction_id);
CREATE INDEX idx_bids_bidder  ON bids (bidder_guild_id);
CREATE INDEX idx_bids_created  ON bids (created_at);
