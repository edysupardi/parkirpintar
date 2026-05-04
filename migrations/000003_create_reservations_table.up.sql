CREATE TYPE reservation_status AS ENUM ('pending', 'confirmed', 'active', 'completed', 'cancelled', 'expired');
CREATE TYPE assignment_mode AS ENUM ('system', 'user_selected');

CREATE TABLE reservations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id       UUID NOT NULL,
    spot_id         UUID NOT NULL REFERENCES spots(id),
    status          reservation_status NOT NULL DEFAULT 'pending',
    assignment_mode assignment_mode NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    confirmed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL,
    check_in_at     TIMESTAMPTZ,
    check_out_at    TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    session_id      UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reservations_driver_id ON reservations (driver_id);
CREATE INDEX idx_reservations_status ON reservations (status);
CREATE INDEX idx_reservations_expires_at ON reservations (expires_at) WHERE status = 'confirmed';
