package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type fakeStore struct {
	events                   []Event
	published, retried, dead int
}

func (s *fakeStore) ClaimDue(context.Context, int, time.Duration, time.Time) ([]Event, error) {
	return s.events, nil
}
func (s *fakeStore) MarkPublished(context.Context, uuid.UUID, time.Time) error {
	s.published++
	return nil
}
func (s *fakeStore) MarkRetry(context.Context, uuid.UUID, int, time.Time, string, string) error {
	s.retried++
	return nil
}
func (s *fakeStore) MarkDead(context.Context, uuid.UUID, int, time.Time, string, string) error {
	s.dead++
	return nil
}

type fakePublisher struct{ err error }

func (p *fakePublisher) Publish(context.Context, Event) error { return p.err }
func (p *fakePublisher) Close() error                         { return nil }

type fakeObserver struct {
	published, dead int
	err             error
}

func (o *fakeObserver) Published(context.Context, Event) error { o.published++; return o.err }
func (o *fakeObserver) Dead(context.Context, Event, error) error {
	o.dead++
	return o.err
}

func testEvent(created time.Time) Event {
	return Event{ID: uuid.New(), CreatedAt: created, Status: StatusProcessing}
}

func TestDispatcherPublishesAndNotifiesObserver(t *testing.T) {
	store := &fakeStore{events: []Event{testEvent(time.Now())}}
	observer := &fakeObserver{}
	dispatcher := NewDispatcher(store, &fakePublisher{}, observer, DispatcherConfig{}, zerolog.Nop())
	if err := dispatcher.Dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.published != 1 || observer.published != 1 {
		t.Fatalf("published=%d observed=%d", store.published, observer.published)
	}
}

func TestDispatcherRetriesBeforeAttemptAndTimeLimits(t *testing.T) {
	store := &fakeStore{events: []Event{testEvent(time.Now())}}
	dispatcher := NewDispatcher(store, &fakePublisher{err: errors.New("offline")}, nil, DispatcherConfig{MaxAttempts: 3, RetryWindow: time.Hour}, zerolog.Nop())
	_ = dispatcher.Dispatch(context.Background())
	if store.retried != 1 || store.dead != 0 {
		t.Fatalf("retried=%d dead=%d", store.retried, store.dead)
	}
}

func TestDispatcherMarksDeadAndNotifiesObserver(t *testing.T) {
	event := testEvent(time.Now().Add(-time.Hour))
	event.AttemptCount = 19
	store := &fakeStore{events: []Event{event}}
	observer := &fakeObserver{}
	dispatcher := NewDispatcher(store, &fakePublisher{err: errors.New("offline")}, observer, DispatcherConfig{MaxAttempts: 20, RetryWindow: 15 * time.Minute}, zerolog.Nop())
	_ = dispatcher.Dispatch(context.Background())
	if store.dead != 1 || observer.dead != 1 {
		t.Fatalf("dead=%d observed=%d", store.dead, observer.dead)
	}
}

func TestDispatcherLeavesPublishedRowLeasedWhenObserverFails(t *testing.T) {
	store := &fakeStore{events: []Event{testEvent(time.Now())}}
	observer := &fakeObserver{err: errors.New("database unavailable")}
	dispatcher := NewDispatcher(store, &fakePublisher{}, observer, DispatcherConfig{}, zerolog.Nop())
	if err := dispatcher.Dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.published != 0 || observer.published != 1 {
		t.Fatalf("published=%d observed=%d", store.published, observer.published)
	}
}

func TestDispatcherLeavesDeadRowLeasedWhenObserverFails(t *testing.T) {
	event := testEvent(time.Now().Add(-time.Hour))
	event.AttemptCount = 19
	store := &fakeStore{events: []Event{event}}
	observer := &fakeObserver{err: errors.New("database unavailable")}
	dispatcher := NewDispatcher(store, &fakePublisher{err: errors.New("offline")}, observer, DispatcherConfig{MaxAttempts: 20, RetryWindow: 15 * time.Minute}, zerolog.Nop())
	_ = dispatcher.Dispatch(context.Background())
	if store.dead != 0 || observer.dead != 1 {
		t.Fatalf("dead=%d observed=%d", store.dead, observer.dead)
	}
}
