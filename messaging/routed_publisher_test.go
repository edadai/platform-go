package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRoutedPublisherIsCreatedWithoutConnecting(t *testing.T) {
	publisher, err := NewRoutedPublisher(RoutedPublisherConfig{
		URL:       "amqp://broker-is-deliberately-unavailable.invalid",
		Exchanges: map[string]string{"edad.commands": "topic"},
	})
	if err != nil {
		t.Fatalf("NewRoutedPublisher() error = %v", err)
	}
	if publisher.connection != nil || publisher.channel != nil {
		t.Fatal("publisher connected eagerly")
	}
}

func TestAwaitConfirmationDrainsAckAfterMandatoryReturn(t *testing.T) {
	confirms := make(chan amqp.Confirmation, 1)
	confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
	timeout := make(chan time.Time)

	if err := awaitConfirmation(context.Background(), timeout, confirms); err != nil {
		t.Fatalf("awaitConfirmation() error = %v", err)
	}
	if len(confirms) != 0 {
		t.Fatal("matching confirmation was not drained")
	}
}

func TestAwaitConfirmationRejectsNackAfterMandatoryReturn(t *testing.T) {
	confirms := make(chan amqp.Confirmation, 1)
	confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: false}
	timeout := make(chan time.Time)

	err := awaitConfirmation(context.Background(), timeout, confirms)
	var publishErr *PublishError
	if !errors.As(err, &publishErr) || publishErr.Code() != "publish_rejected" {
		t.Fatalf("awaitConfirmation() error = %v", err)
	}
}

func TestUnroutableDoesNotInvalidateHealthyConnection(t *testing.T) {
	if invalidatesConnection(&PublishError{code: "unroutable", detail: "returned"}) {
		t.Fatal("unroutable message should not invalidate the connection")
	}
	if !invalidatesConnection(errors.New("unknown transport error")) {
		t.Fatal("unknown transport error should invalidate the connection")
	}
}
