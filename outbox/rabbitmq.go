package outbox

import (
	"bytes"
	"context"
	"encoding/json"

	platformmessaging "github.com/edadai/platform-go/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitPublisher struct {
	publisher *platformmessaging.RoutedPublisher
}

func NewRabbitPublisher(config platformmessaging.RoutedPublisherConfig) (*RabbitPublisher, error) {
	publisher, err := platformmessaging.NewRoutedPublisher(config)
	if err != nil {
		return nil, err
	}
	return &RabbitPublisher{publisher: publisher}, nil
}

func (p *RabbitPublisher) Publish(ctx context.Context, event Event) error {
	var headers map[string]any
	decoder := json.NewDecoder(bytes.NewReader([]byte(event.HeadersJSON)))
	decoder.UseNumber()
	if err := decoder.Decode(&headers); err != nil {
		return err
	}
	table := amqp.Table{}
	for key, value := range headers {
		table[key] = amqpHeaderValue(value)
	}
	return p.publisher.Publish(ctx, event.Exchange, event.RoutingKey, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: event.ID.String(),
		Type: event.MessageType, CorrelationId: event.CorrelationID, Timestamp: event.CreatedAt,
		Headers: table, Body: []byte(event.PayloadJSON),
	})
}

func amqpHeaderValue(value any) any {
	if number, ok := value.(json.Number); ok {
		if integer, err := number.Int64(); err == nil {
			return integer
		}
		if decimal, err := number.Float64(); err == nil {
			return decimal
		}
	}
	return value
}

func (p *RabbitPublisher) Close() error { return p.publisher.Close() }
