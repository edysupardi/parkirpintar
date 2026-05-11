CREATE TABLE parking_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reservation_id  UUID NOT NULL REFERENCES reservations(id),
    driver_id       UUID NOT NULL,
    spot_id         UUID NOT NULL REFERENCES spots(id),
    check_in_at     TIMESTAMPTZ NOT NULL,
    check_out_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_parking_sessions_reservation_id ON parking_sessions (reservation_id);
