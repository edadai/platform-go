package di

import "context"

// Lifetime controls how long a resolved value lives inside the registry.
type Lifetime uint8

const (
	Singleton Lifetime = iota
)

// Provider constructs one dependency.
type Provider struct {
	Name     string
	Lifetime Lifetime

	Build   func(Resolver) (any, error)
	Cleanup func(context.Context, any) error
}

// Resolver is intentionally small.
// Constructors receive this instead of knowing about Registry.
type Resolver interface {
	Resolve(name string) (any, error)
}

// As resolves name and asserts it to T.
func As[T any](r Resolver, name string) (T, error) {
	var zero T

	value, err := r.Resolve(name)
	if err != nil {
		return zero, err
	}

	typed, ok := value.(T)
	if !ok {
		return zero, &Error{
			Kind:     ErrTypeMismatch,
			Provider: name,
			Cause:    fmtTypeMismatch[T](value),
		}
	}

	return typed, nil
}
