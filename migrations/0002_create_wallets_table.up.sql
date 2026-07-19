CREATE TABLE wallets (
    id              BIGSERIAL PRIMARY KEY,
    guild_id        BIGINT NOT NULL,
    total_balance   BIGINT NOT NULL DEFAULT 0,
    reserved_amount BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ,
    CONSTRAINT chk_wallet_nonneg
        CHECK (total_balance >= 0 AND reserved_amount >= 0 AND reserved_amount <= total_balance)
);

CREATE UNIQUE INDEX idx_wallets_guild_id ON wallets (guild_id);
