CREATE TABLE IF NOT EXISTS notification_logs (
    id VARCHAR(36) PRIMARY KEY,
    driver_id VARCHAR(36) NOT NULL,
    channel VARCHAR(20) NOT NULL,
    template_id VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'sent',
    error_message TEXT,
    sent_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notification_logs_driver_id ON notification_logs(driver_id);
CREATE INDEX idx_notification_logs_sent_at ON notification_logs(sent_at DESC);
