package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidMessage = errors.New("invalid outbox message")

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Enqueue(ctx context.Context, tx *gorm.DB, message Message) (*Event, error) {
	if message.DeduplicationKey == "" || message.AggregateType == "" || message.AggregateID == uuid.Nil ||
		message.MessageType == "" || message.Exchange == "" || message.RoutingKey == "" || message.CorrelationID == "" {
		return nil, ErrInvalidMessage
	}
	payload, err := json.Marshal(message.Payload)
	if err != nil {
		return nil, err
	}
	headers, err := json.Marshal(message.Headers)
	if err != nil {
		return nil, err
	}
	now := message.OccurredAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := message.ID
	if id == uuid.Nil {
		id = uuid.Must(uuid.NewV7())
	}
	event := &Event{
		ID: id, DeduplicationKey: message.DeduplicationKey, AggregateType: message.AggregateType,
		AggregateID: message.AggregateID, MessageType: message.MessageType, Exchange: message.Exchange,
		RoutingKey: message.RoutingKey, CorrelationID: message.CorrelationID, PayloadJSON: string(payload),
		HeadersJSON: string(headers), Status: StatusPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	database := r.db
	if tx != nil {
		database = tx
	}
	if err := database.WithContext(ctx).Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *Repository) ClaimDue(ctx context.Context, limit int, lease time.Duration, now time.Time) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	now = now.UTC()
	var events []Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("((status IN ? AND next_attempt_at <= ?) OR (status = ? AND lease_until <= ?))",
				[]string{StatusPending, StatusRetry}, now, StatusProcessing, now).
			Order("next_attempt_at ASC, created_at ASC").Limit(limit)
		if err := query.Find(&events).Error; err != nil || len(events) == 0 {
			return err
		}
		ids := make([]uuid.UUID, len(events))
		for index := range events {
			ids[index] = events[index].ID
		}
		until := now.Add(lease)
		return tx.Model(&Event{}).Where("id IN ?", ids).Updates(map[string]any{
			"status": StatusProcessing, "lease_until": until, "updated_at": now,
		}).Error
	})
	return events, err
}

func (r *Repository) MarkPublished(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Model(&Event{}).Where("id = ?", id).Updates(map[string]any{
		"status": StatusPublished, "published_at": at.UTC(), "lease_until": nil,
		"last_error_code": "", "last_error_message": "", "updated_at": at.UTC(),
	}).Error
}

func (r *Repository) MarkRetry(ctx context.Context, id uuid.UUID, attempts int, next time.Time, code, message string) error {
	return r.updateFailure(ctx, id, StatusRetry, attempts, next, code, message)
}

func (r *Repository) MarkDead(ctx context.Context, id uuid.UUID, attempts int, at time.Time, code, message string) error {
	return r.updateFailure(ctx, id, StatusDead, attempts, at, code, message)
}

func (r *Repository) updateFailure(ctx context.Context, id uuid.UUID, status string, attempts int, next time.Time, code, message string) error {
	if len(message) > 2000 {
		message = message[:2000]
	}
	return r.db.WithContext(ctx).Model(&Event{}).Where("id = ?", id).Updates(map[string]any{
		"status": status, "attempt_count": attempts, "next_attempt_at": next.UTC(), "lease_until": nil,
		"last_error_code": code, "last_error_message": message, "updated_at": time.Now().UTC(),
	}).Error
}

func (r *Repository) Reactivate(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Model(&Event{}).Where("id = ? AND status = ?", id, StatusDead).Updates(map[string]any{
		"status": StatusPending, "attempt_count": 0, "next_attempt_at": at.UTC(), "lease_until": nil,
		"last_error_code": "", "last_error_message": "", "published_at": nil, "updated_at": at.UTC(),
	}).Error
}

func (r *Repository) PendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Event{}).Where("status IN ?", []string{StatusPending, StatusRetry, StatusProcessing}).Count(&count).Error
	return count, err
}

func (r *Repository) DeadCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Event{}).Where("status = ?", StatusDead).Count(&count).Error
	return count, err
}

func (r *Repository) OldestPendingAge(ctx context.Context, now time.Time) (time.Duration, error) {
	var event Event
	err := r.db.WithContext(ctx).Where("status IN ?", []string{StatusPending, StatusRetry, StatusProcessing}).Order("created_at ASC").First(&event).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return now.UTC().Sub(event.CreatedAt), nil
}
