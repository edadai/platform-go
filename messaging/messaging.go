package messaging

import (
	"context"
	"encoding/json"
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
}

func NewClient(_ context.Context, cfg Config, bindings []QueueBinding, log zerolog.Logger) (*Client, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := channel.ExchangeDeclare(cfg.NormalExchange(), "topic", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}
	if err := channel.ExchangeDeclare(cfg.DLX(), "topic", true, false, false, false, nil); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	for _, binding := range bindings {
		if err := declareAndBind(channel, cfg, binding); err != nil {
			_ = channel.Close()
			_ = conn.Close()
			return nil, err
		}
	}

	log.Info().Str("exchange", cfg.NormalExchange()).Str("dlx", cfg.DLX()).Msg("rabbitmq connected")
	return &Client{connection: conn, channel: channel, config: cfg}, nil
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
	return c.channel.PublishWithContext(ctx, c.config.NormalExchange(), routingKey, false, false, publishing)
}

func (c *Client) Consume(queue string) (<-chan amqp.Delivery, error) {
	return c.channel.Consume(queue, "", false, false, false, false, nil)
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
