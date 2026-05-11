CREATE TABLE drivers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(100) NOT NULL,
    email         VARCHAR(255) NOT NULL UNIQUE,
    phone         VARCHAR(20) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    vehicle_type  vehicle_type NOT NULL DEFAULT 'car',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_drivers_email ON drivers (email);
