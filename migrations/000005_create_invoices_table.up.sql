CREATE TYPE invoice_status AS ENUM ('draft', 'pending_payment', 'paid', 'void');

CREATE TABLE invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL,
    reservation_id  UUID NOT NULL REFERENCES reservations(id),
    driver_id       UUID NOT NULL,
    booking_fee     BIGINT NOT NULL DEFAULT 0,
    parking_fee     BIGINT NOT NULL DEFAULT 0,
    overnight_fee   BIGINT NOT NULL DEFAULT 0,
    total_amount    BIGINT NOT NULL DEFAULT 0,
    billed_hours    SMALLINT NOT NULL DEFAULT 0,
    is_overnight    BOOLEAN NOT NULL DEFAULT FALSE,
    duration_mins   BIGINT NOT NULL DEFAULT 0,
    status          invoice_status NOT NULL DEFAULT 'pending_payment',
    gateway_tx_id   VARCHAR(255),
    payment_method  VARCHAR(50),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at         TIMESTAMPTZ
);

CREATE INDEX idx_invoices_session_id ON invoices (session_id);
CREATE INDEX idx_invoices_reservation_id ON invoices (reservation_id);
CREATE INDEX idx_invoices_driver_id ON invoices (driver_id);
