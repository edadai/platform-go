package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type PublishError struct{ code, detail string }

func (e *PublishError) Error() string { return e.detail }
func (e *PublishError) Code() string  { return e.code }

type RoutedPublisherConfig struct {
	URL string
	// Exchanges is an allowlist from exchange name to RabbitMQ exchange kind (usually "topic").
	Exchanges      map[string]string
	ConfirmTimeout time.Duration
}

type RoutedPublisher struct {
	config     RoutedPublisherConfig
	mu         sync.Mutex
	connection *amqp.Connection
	channel    *amqp.Channel
	confirms   <-chan amqp.Confirmation
	returns    <-chan amqp.Return
}

func NewRoutedPublisher(config RoutedPublisherConfig) (*RoutedPublisher, error) {
	if config.URL == "" || len(config.Exchanges) == 0 {
		return nil, errors.New("routed publisher URL and exchanges are required")
	}
	// Connection is deliberately lazy. A broker outage must not prevent an API or
	// service from starting and accepting work into its transactional outbox.
	return &RoutedPublisher{config: config}, nil
}

func (p *RoutedPublisher) connect() error {
	connection, err := amqp.Dial(p.config.URL)
	if err != nil {
		return &PublishError{code: "connection_failed", detail: err.Error()}
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return &PublishError{code: "channel_failed", detail: err.Error()}
	}
	for exchange, kind := range p.config.Exchanges {
		if kind == "" {
			kind = "topic"
		}
		if err := channel.ExchangeDeclare(exchange, kind, true, false, false, false, nil); err != nil {
			_ = channel.Close()
			_ = connection.Close()
			return &PublishError{code: "exchange_declare_failed", detail: err.Error()}
		}
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return &PublishError{code: "confirmation_setup_failed", detail: err.Error()}
	}
	p.connection, p.channel = connection, channel
	p.confirms = channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	p.returns = channel.NotifyReturn(make(chan amqp.Return, 1))
	return nil
}

func (p *RoutedPublisher) Publish(ctx context.Context, exchange, routingKey string, message amqp.Publishing) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, allowed := p.config.Exchanges[exchange]; !allowed {
		return &PublishError{code: "exchange_not_allowed", detail: fmt.Sprintf("exchange %q is not allowed", exchange)}
	}
	if p.channel == nil || p.channel.IsClosed() {
		if err := p.reconnect(); err != nil {
			return err
		}
	}
	err := p.publishOnce(ctx, exchange, routingKey, message)
	if err != nil && invalidatesConnection(err) {
		p.invalidate()
	}
	return err
}

func (p *RoutedPublisher) publishOnce(ctx context.Context, exchange, routingKey string, message amqp.Publishing) error {
	if err := p.channel.PublishWithContext(ctx, exchange, routingKey, true, false, message); err != nil {
		return &PublishError{code: "publish_failed", detail: err.Error()}
	}
	timeout := p.config.ConfirmTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case returned, ok := <-p.returns:
		if ok {
			return &PublishError{code: "unroutable", detail: fmt.Sprintf("message returned by broker: %d %s", returned.ReplyCode, returned.ReplyText)}
		}
	case confirmation, ok := <-p.confirms:
		if !ok {
			return &PublishError{code: "confirmation_closed", detail: "publisher confirmation channel closed"}
		}
		if !confirmation.Ack {
			return &PublishError{code: "publish_rejected", detail: "broker rejected published message"}
		}
		select {
		case returned := <-p.returns:
			return &PublishError{code: "unroutable", detail: fmt.Sprintf("message returned by broker: %d %s", returned.ReplyCode, returned.ReplyText)}
		default:
			return nil
		}
	case <-ctx.Done():
		return &PublishError{code: "publish_context_done", detail: ctx.Err().Error()}
	case <-timer.C:
		return &PublishError{code: "confirmation_timeout", detail: "timed out waiting for publisher confirmation"}
	}
	return nil
}

func (p *RoutedPublisher) reconnect() error {
	p.invalidate()
	return p.connect()
}

func (p *RoutedPublisher) invalidate() {
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.connection != nil {
		_ = p.connection.Close()
	}
	p.channel = nil
	p.connection = nil
	p.confirms = nil
	p.returns = nil
}

func invalidatesConnection(err error) bool {
	var publishErr *PublishError
	if !errors.As(err, &publishErr) {
		return true
	}
	switch publishErr.Code() {
	case "exchange_not_allowed", "unroutable", "publish_rejected":
		return false
	default:
		return true
	}
}

func (p *RoutedPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.connection != nil {
		return p.connection.Close()
	}
	return nil
}
