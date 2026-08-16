package outbox

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Publisher interface {
	Publish(context.Context, Event) error
	Close() error
}

type Observer interface {
	Published(context.Context, Event) error
	Dead(context.Context, Event, error) error
}

type NopObserver struct{}

func (NopObserver) Published(context.Context, Event) error   { return nil }
func (NopObserver) Dead(context.Context, Event, error) error { return nil }

type DispatcherConfig struct {
	BatchSize    int
	PollInterval time.Duration
	Lease        time.Duration
	MaxAttempts  int
	RetryWindow  time.Duration
	MaxBackoff   time.Duration
}

type Store interface {
	ClaimDue(context.Context, int, time.Duration, time.Time) ([]Event, error)
	MarkPublished(context.Context, uuid.UUID, time.Time) error
	MarkRetry(context.Context, uuid.UUID, int, time.Time, string, string) error
	MarkDead(context.Context, uuid.UUID, int, time.Time, string, string) error
}

type Dispatcher struct {
	repo      Store
	publisher Publisher
	observer  Observer
	config    DispatcherConfig
	log       zerolog.Logger
}

func NewDispatcher(repo Store, publisher Publisher, observer Observer, config DispatcherConfig, log zerolog.Logger) *Dispatcher {
	if observer == nil {
		observer = NopObserver{}
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.Lease <= 0 {
		config.Lease = 5 * time.Minute
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 20
	}
	if config.RetryWindow <= 0 {
		config.RetryWindow = 15 * time.Minute
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = time.Minute
	}
	return &Dispatcher{repo: repo, publisher: publisher, observer: observer, config: config, log: log}
}

func (d *Dispatcher) Close() error { return d.publisher.Close() }

func (d *Dispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := d.Dispatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			d.log.Error().Err(err).Msg("outbox dispatch failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context) error {
	now := time.Now().UTC()
	events, err := d.repo.ClaimDue(ctx, d.config.BatchSize, d.config.Lease, now)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := d.publishOne(ctx, event, now); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (d *Dispatcher) publishOne(ctx context.Context, event Event, now time.Time) error {
	err := d.publisher.Publish(ctx, event)
	if err == nil {
		if observerErr := d.observer.Published(ctx, event); observerErr != nil {
			d.log.Error().Err(observerErr).Str("outbox_id", event.ID.String()).Msg("outbox published observer failed")
			// Leave the row processing. Lease recovery will publish the idempotent
			// message again and retry the lifecycle callback.
			return observerErr
		}
		if markErr := d.repo.MarkPublished(ctx, event.ID, time.Now().UTC()); markErr != nil {
			return markErr
		}
		return nil
	}
	attempts := event.AttemptCount + 1
	code := errorCode(err)
	exhausted := attempts >= d.config.MaxAttempts || now.Sub(event.CreatedAt) >= d.config.RetryWindow
	if exhausted {
		event.AttemptCount, event.Status = attempts, StatusDead
		if observerErr := d.observer.Dead(ctx, event, err); observerErr != nil {
			d.log.Error().Err(observerErr).Str("outbox_id", event.ID.String()).Msg("outbox dead observer failed")
			// As above, lease recovery retries the callback instead of leaving a
			// terminal outbox row with stale aggregate state.
			return observerErr
		}
		if markErr := d.repo.MarkDead(ctx, event.ID, attempts, now, code, err.Error()); markErr != nil {
			return markErr
		}
		return err
	}
	next := now.Add(retryDelay(attempts, d.config.MaxBackoff))
	if markErr := d.repo.MarkRetry(ctx, event.ID, attempts, next, code, err.Error()); markErr != nil {
		return markErr
	}
	d.log.Warn().Err(err).Str("outbox_id", event.ID.String()).Int("attempt", attempts).Time("next_attempt_at", next).Msg("outbox publish retry scheduled")
	return err
}

func retryDelay(attempt int, maximum time.Duration) time.Duration {
	seconds := math.Min(math.Pow(2, float64(max(attempt-1, 0))), maximum.Seconds())
	base := time.Duration(seconds * float64(time.Second))
	jitterRange := max(base/4, time.Millisecond)
	return min(base+time.Duration(rand.Int64N(int64(jitterRange))), maximum)
}

type codedError interface{ Code() string }

func errorCode(err error) string {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.Code()
	}
	return "publish_failed"
}
