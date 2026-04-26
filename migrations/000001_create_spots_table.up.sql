CREATE TYPE vehicle_type AS ENUM ('car', 'motorcycle');
CREATE TYPE spot_status AS ENUM ('available', 'reserved', 'occupied', 'maintenance');

CREATE TABLE spots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    floor        SMALLINT NOT NULL CHECK (floor BETWEEN 1 AND 5),
    number       SMALLINT NOT NULL,
    vehicle_type vehicle_type NOT NULL,
    status       spot_status NOT NULL DEFAULT 'available',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (floor, number, vehicle_type)
);

CREATE INDEX idx_spots_vehicle_type_status ON spots (vehicle_type, status);
