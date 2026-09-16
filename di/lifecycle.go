package di

import (
	"context"
	"errors"
)

type cleanup struct {
	name string
	fn   func(context.Context) error
}

type Lifecycle struct {
	cleanups []cleanup
}

func (l *Lifecycle) Add(name string, fn func(context.Context) error) {
	if fn == nil {
		return
	}

	l.cleanups = append(l.cleanups, cleanup{
		name: name,
		fn:   fn,
	})
}

func (l *Lifecycle) Close(ctx context.Context) error {
	var errs []error

	// Reverse construction order.
	for i := len(l.cleanups) - 1; i >= 0; i-- {
		if err := l.cleanups[i].fn(ctx); err != nil {
			errs = append(errs,
				&Error{
					Kind:     ErrCleanup,
					Provider: l.cleanups[i].name,
					Cause:    err,
				},
			)
		}
	}

	return errors.Join(errs...)
}
