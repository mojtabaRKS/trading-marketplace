CREATE TABLE oracle_prices (
    id          BIGSERIAL PRIMARY KEY,
    item_id     BIGINT NOT NULL,
    price       BIGINT NOT NULL,
    source      VARCHAR(32),
    observed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ,
    CONSTRAINT chk_oracle_price_pos CHECK (price > 0) -- only validated (good) prices are stored
);

CREATE INDEX idx_oracle_item    ON oracle_prices (item_id);
CREATE INDEX idx_oracle_created ON oracle_prices (created_at);
