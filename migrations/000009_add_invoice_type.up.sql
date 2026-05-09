CREATE TYPE invoice_type AS ENUM ('booking_fee', 'parking_session');

ALTER TABLE invoices ADD COLUMN type invoice_type NOT NULL DEFAULT 'parking_session';
ALTER TABLE invoices ALTER COLUMN session_id DROP NOT NULL;

CREATE INDEX idx_invoices_type ON invoices (type);
