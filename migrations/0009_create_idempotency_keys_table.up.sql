CREATE TABLE idempotency_keys (
    key             VARCHAR(255) PRIMARY KEY,
    request_hash    VARCHAR(64) NOT NULL,
    response_status INT         NOT NULL,
    response_body   BYTEA,
    created_at      TIMESTAMPTZ
);
