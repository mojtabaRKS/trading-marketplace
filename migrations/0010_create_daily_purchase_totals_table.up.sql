CREATE TABLE daily_purchase_totals (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL,
    day         DATE   NOT NULL,
    total_spent BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT chk_daily_spent_nonneg CHECK (total_spent >= 0)
);

CREATE UNIQUE INDEX idx_guild_day ON daily_purchase_totals (guild_id, day);
