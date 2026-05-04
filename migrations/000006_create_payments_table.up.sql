CREATE TYPE transaction_status AS ENUM ('pending', 'settled', 'failed', 'expired', 'refunded');

CREATE TABLE transactions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    driver_id       UUID NOT NULL,
    gateway_tx_id   VARCHAR(255) NOT NULL UNIQUE,
    payment_method  VARCHAR(50) NOT NULL,
    status          transaction_status NOT NULL DEFAULT 'pending',
    amount          BIGINT NOT NULL,
    payment_url     TEXT,
    qr_string       TEXT,
    va_number       VARCHAR(50),
    va_bank         VARCHAR(20),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at         TIMESTAMPTZ,
    expired_at      TIMESTAMPTZ
);

CREATE INDEX idx_transactions_invoice_id ON transactions (invoice_id);
CREATE INDEX idx_transactions_gateway_tx_id ON transactions (gateway_tx_id);
