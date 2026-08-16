package outbox

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusRetry      = "retry"
	StatusPublished  = "published"
	StatusDead       = "dead"
)

// Event is a final, transport-ready message stored in the same database transaction as its
// aggregate. Every service owns an outbox_events table in its own database.
type Event struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	DeduplicationKey string     `gorm:"not null;uniqueIndex" json:"deduplication_key"`
	AggregateType    string     `gorm:"not null;index" json:"aggregate_type"`
	AggregateID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"aggregate_id"`
	MessageType      string     `gorm:"not null;index" json:"message_type"`
	Exchange         string     `gorm:"not null" json:"exchange"`
	RoutingKey       string     `gorm:"not null" json:"routing_key"`
	CorrelationID    string     `gorm:"not null;index" json:"correlation_id"`
	PayloadJSON      string     `gorm:"type:jsonb;not null" json:"payload_json"`
	HeadersJSON      string     `gorm:"type:jsonb;not null;default:'{}'" json:"headers_json"`
	Status           string     `gorm:"not null;index" json:"status"`
	AttemptCount     int        `gorm:"not null;default:0" json:"attempt_count"`
	NextAttemptAt    time.Time  `gorm:"not null;index" json:"next_attempt_at"`
	LeaseUntil       *time.Time `json:"lease_until,omitempty"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	LastErrorMessage string     `json:"last_error_message,omitempty"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	CreatedAt        time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updated_at"`
}

func (Event) TableName() string { return "outbox_events" }

type Message struct {
	ID               uuid.UUID
	DeduplicationKey string
	AggregateType    string
	AggregateID      uuid.UUID
	MessageType      string
	Exchange         string
	RoutingKey       string
	CorrelationID    string
	Payload          any
	Headers          map[string]any
	OccurredAt       time.Time
}
