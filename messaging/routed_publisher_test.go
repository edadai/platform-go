package messaging

import (
	"errors"
	"testing"
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

func TestUnroutableDoesNotInvalidateHealthyConnection(t *testing.T) {
	if invalidatesConnection(&PublishError{code: "unroutable", detail: "returned"}) {
		t.Fatal("unroutable message should not invalidate the connection")
	}
	if !invalidatesConnection(errors.New("unknown transport error")) {
		t.Fatal("unknown transport error should invalidate the connection")
	}
}
