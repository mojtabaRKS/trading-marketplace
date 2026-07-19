CREATE TABLE guilds (
    id                 BIGSERIAL PRIMARY KEY,
    name               VARCHAR(120) NOT NULL,
    daily_purchase_cap BIGINT       NOT NULL DEFAULT 0, -- minor units; 0 = unlimited
    created_at         TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ,
    CONSTRAINT chk_guild_cap_nonneg CHECK (daily_purchase_cap >= 0)
);

CREATE UNIQUE INDEX idx_guilds_name ON guilds (name);
