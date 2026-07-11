package messaging

import (
	"encoding/json"
	"time"
)

const (
	DefaultExchange           = "edad.events"
	DefaultDeadLetterExchange = "edad.events.dlx"

	NotificationRequestedRoutingKey = "notification.requested.v1"
	NotificationDeliveryRoutingKey  = "notification.delivery.requested.v1"
)

type EventEnvelope struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	AggregateID string          `json:"aggregate_id,omitempty"`
	Data        json.RawMessage `json:"data"`
}

type NotificationRequested struct {
	EventID     string            `json:"event_id"`
	EventType   string            `json:"event_type"`
	Channel     string            `json:"channel"`
	Priority    string            `json:"priority,omitempty"`
	Recipient   string            `json:"recipient"`
	Subject     string            `json:"subject"`
	TemplateKey string            `json:"template_key"`
	Data        map[string]string `json:"data"`
}
