package di

import "fmt"

type ErrorKind string

const (
	ErrProviderNotFound ErrorKind = "provider_not_found"
	ErrDuplicate        ErrorKind = "duplicate_provider"
	ErrCircular         ErrorKind = "circular_dependency"
	ErrBuild            ErrorKind = "provider_build"
	ErrCleanup          ErrorKind = "provider_cleanup"
	ErrTypeMismatch     ErrorKind = "type_mismatch"
	ErrClosed           ErrorKind = "registry_closed"
)

type Error struct {
	Kind     ErrorKind
	Provider string
	Cause    error
}

func (e *Error) Error() string {
	switch e.Kind {
	case ErrProviderNotFound:
		return fmt.Sprintf("dependency %q not registered", e.Provider)

	case ErrDuplicate:
		return fmt.Sprintf("provider %q already registered", e.Provider)

	case ErrCircular:
		return fmt.Sprintf("circular dependency detected while resolving %q", e.Provider)

	case ErrBuild:
		return fmt.Sprintf("failed to build provider %q: %v", e.Provider, e.Cause)

	case ErrCleanup:
		return fmt.Sprintf("failed to close provider %q: %v", e.Provider, e.Cause)

	case ErrTypeMismatch:
		return fmt.Sprintf("dependency %q has unexpected type: %v", e.Provider, e.Cause)

	case ErrClosed:
		return "di registry is closed"

	default:
		return fmt.Sprintf("di error for provider %q: %v", e.Provider, e.Cause)
	}
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func fmtTypeMismatch[T any](value any) error {
	var zero T
	return fmt.Errorf("want %T, got %T", zero, value)
}
