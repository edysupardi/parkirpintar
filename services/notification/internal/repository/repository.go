package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type NotificationLog struct {
	ID           string
	DriverID     string
	Channel      string
	TemplateID   string
	Title        string
	Body         string
	Status       string
	ErrorMessage string
	SentAt       time.Time
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Insert(ctx context.Context, log NotificationLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_logs (id, driver_id, channel, template_id, title, body, status, error_message, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		log.ID, log.DriverID, log.Channel, log.TemplateID, log.Title, log.Body, log.Status, log.ErrorMessage, log.SentAt)
	return err
}

func (r *Repository) GetByDriverID(ctx context.Context, driverID string, limit, offset int32) ([]NotificationLog, int32, error) {
	if limit <= 0 {
		limit = 20
	}

	var total int32
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_logs WHERE driver_id = $1`, driverID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, driver_id, channel, template_id, title, body, status, error_message, sent_at
		FROM notification_logs
		WHERE driver_id = $1
		ORDER BY sent_at DESC
		LIMIT $2 OFFSET $3`, driverID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []NotificationLog
	for rows.Next() {
		var l NotificationLog
		if err := rows.Scan(&l.ID, &l.DriverID, &l.Channel, &l.TemplateID, &l.Title, &l.Body, &l.Status, &l.ErrorMessage, &l.SentAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}

	return logs, total, nil
}
