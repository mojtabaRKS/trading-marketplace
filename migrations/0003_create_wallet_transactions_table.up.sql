CREATE TABLE wallet_transactions (
    id         BIGSERIAL PRIMARY KEY,
    wallet_id  BIGINT      NOT NULL,
    guild_id   BIGINT      NOT NULL,
    type       VARCHAR(16) NOT NULL, -- reserve|release|debit|credit
    amount     BIGINT      NOT NULL,
    ref_type   VARCHAR(16),          -- listing|auction|bid
    ref_id     BIGINT,
    created_at TIMESTAMPTZ,
    CONSTRAINT chk_wallet_tx_type CHECK (type IN ('reserve','release','debit','credit'))
);

CREATE INDEX idx_wallet_tx_wallet  ON wallet_transactions (wallet_id);
CREATE INDEX idx_wallet_tx_guild   ON wallet_transactions (guild_id);
CREATE INDEX idx_wallet_tx_created ON wallet_transactions (created_at);
