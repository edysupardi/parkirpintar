CREATE TABLE location_updates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    driver_id       UUID NOT NULL,
    session_id      UUID,
    reservation_id  UUID,
    latitude        DOUBLE PRECISION NOT NULL,
    longitude       DOUBLE PRECISION NOT NULL,
    accuracy        REAL,
    recorded_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_location_updates_driver_id ON location_updates (driver_id, recorded_at DESC);
