package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
)

type Config struct {
	URL                string
	Exchange           string
	DeadLetterExchange string
}

func (c Config) NormalExchange() string {
	if c.Exchange == "" {
		return DefaultExchange
	}
	return c.Exchange
}

func (c Config) DLX() string {
	if c.DeadLetterExchange == "" {
		return DefaultDeadLetterExchange
	}
	return c.DeadLetterExchange
}

type QueueBinding struct {
	Queue      string
	RoutingKey string
}

type Client struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	config     Config
	bindings   []QueueBinding
	log        zerolog.Logger
	publishMu  sync.Mutex
	confirms   <-chan amqp.Confirmation
}

func NewClient(_ context.Context, cfg Config, bindings []QueueBinding, log zerolog.Logger) (*Client, error) {
	client := &Client{config: cfg, bindings: append([]QueueBinding(nil), bindings...), log: log}
	if err := client.connect(); err != nil {
		return nil, err
	}
	log.Info().Str("exchange", cfg.NormalExchange()).Str("dlx", cfg.DLX()).Msg("rabbitmq connected")
	return client, nil
}

func (c *Client) connect() error {
	cfg := c.config
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}

	if err := channel.ExchangeDeclare(cfg.NormalExchange(), "topic", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}
	if err := channel.ExchangeDeclare(cfg.DLX(), "topic", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}

	for _, binding := range c.bindings {
		if err := declareAndBind(channel, cfg, binding); err != nil {
			_ = channel.Close()
			_ = conn.Close()
			return err
		}
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return err
	}
	c.connection = conn
	c.channel = channel
	c.confirms = channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	return nil
}

func declareAndBind(channel *amqp.Channel, cfg Config, binding QueueBinding) error {
	queue, err := channel.QueueDeclare(binding.Queue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": cfg.DLX(),
	})
	if err != nil {
		return err
	}
	return channel.QueueBind(queue.Name, binding.RoutingKey, cfg.NormalExchange(), false, nil)
}

func (c *Client) PublishJSON(ctx context.Context, routingKey string, messageID string, messageType string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Publish(ctx, routingKey, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID,
		Type:         messageType,
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
}

func (c *Client) Publish(ctx context.Context, routingKey string, publishing amqp.Publishing) error {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if err := c.publishOnce(ctx, routingKey, publishing); err == nil {
		return nil
	} else {
		c.log.Warn().Err(err).Msg("rabbitmq publish failed; reconnecting")
		if reconnectErr := c.reconnect(); reconnectErr != nil {
			return errors.Join(err, reconnectErr)
		}
		// The first publication may have reached the broker before its confirm
		// channel closed. Stable message IDs make this retry safely deduplicable.
		return c.publishOnce(ctx, routingKey, publishing)
	}
}

func (c *Client) publishOnce(ctx context.Context, routingKey string, publishing amqp.Publishing) error {
	if c.channel == nil || c.channel.IsClosed() {
		return errors.New("rabbitmq channel is closed")
	}
	if err := c.channel.PublishWithContext(ctx, c.config.NormalExchange(), routingKey, false, false, publishing); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case confirmation, ok := <-c.confirms:
		if !ok {
			return errors.New("rabbitmq publisher confirmation channel closed")
		}
		if !confirmation.Ack {
			return errors.New("rabbitmq rejected published message")
		}
		return nil
	}
}

func (c *Client) reconnect() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.connection != nil {
		_ = c.connection.Close()
	}
	if err := c.connect(); err != nil {
		return err
	}
	c.log.Info().Str("exchange", c.config.NormalExchange()).Msg("rabbitmq publisher reconnected")
	return nil
}

func (c *Client) Consume(queue string) (<-chan amqp.Delivery, error) {
	if err := c.channel.Qos(50, 0, false); err != nil {
		return nil, err
	}
	return c.channel.Consume(queue, "", false, false, false, false, nil)
}

// ConsumeResilient relays deliveries across RabbitMQ channel reconnects. The
// caller keeps one stable channel and retains explicit Ack/Nack control.
func (c *Client) ConsumeResilient(ctx context.Context, queue string) (<-chan amqp.Delivery, error) {
	source, err := c.Consume(queue)
	if err != nil {
		return nil, err
	}
	output := make(chan amqp.Delivery)
	go func() {
		defer close(output)
		for {
			for delivery := range source {
				select {
				case <-ctx.Done():
					return
				case output <- delivery:
				}
			}
			if ctx.Err() != nil {
				return
			}
			c.log.Warn().Str("queue", queue).Msg("rabbitmq consumer channel closed; reconnecting")
			for {
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				c.publishMu.Lock()
				reconnectErr := c.reconnect()
				if reconnectErr == nil {
					source, reconnectErr = c.Consume(queue)
				}
				c.publishMu.Unlock()
				if reconnectErr == nil {
					c.log.Info().Str("queue", queue).Msg("rabbitmq consumer reconnected")
					break
				}
				c.log.Error().Err(reconnectErr).Str("queue", queue).Msg("rabbitmq consumer reconnect failed")
			}
		}
	}()
	return output, nil
}

func (c *Client) Close() error {
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.connection != nil {
		return c.connection.Close()
	}
	return nil
}
