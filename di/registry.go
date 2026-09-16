package di

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type Registry struct {
	mu sync.Mutex

	providers map[string]Provider
	instances map[string]any
	building  map[string]*buildState
	lifecycle Lifecycle
	closed    bool
}

type buildState struct {
	done  chan struct{}
	value any
	err   error
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		instances: make(map[string]any),
		building:  make(map[string]*buildState),
	}
}

func (r *Registry) Provide(provider Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return &Error{Kind: ErrClosed}
	}

	if provider.Name == "" {
		return fmt.Errorf("provider name cannot be empty")
	}

	if provider.Build == nil {
		return fmt.Errorf("provider %q has no builder", provider.Name)
	}

	if _, exists := r.providers[provider.Name]; exists {
		return &Error{
			Kind:     ErrDuplicate,
			Provider: provider.Name,
		}
	}

	r.providers[provider.Name] = provider

	return nil
}

type resolver struct {
	registry *Registry
	stack    []string
}

func (r *resolver) Resolve(name string) (any, error) {
	return r.registry.resolve(name, r.stack)
}

// Resolve constructs or returns the named dependency.
func (r *Registry) Resolve(name string) (any, error) {
	return r.resolve(name, nil)
}

func (r *Registry) resolve(name string, stack []string) (any, error) {
	r.mu.Lock()

	if r.closed {
		r.mu.Unlock()
		return nil, &Error{Kind: ErrClosed, Provider: name}
	}

	if instance, ok := r.instances[name]; ok {
		r.mu.Unlock()
		return instance, nil
	}

	provider, ok := r.providers[name]
	if !ok {
		r.mu.Unlock()

		return nil, &Error{
			Kind:     ErrProviderNotFound,
			Provider: name,
		}
	}

	for _, current := range stack {
		if current == name {
			r.mu.Unlock()

			return nil, &Error{
				Kind:     ErrCircular,
				Provider: name,
			}
		}
	}

	if state, ok := r.building[name]; ok {
		r.mu.Unlock()

		<-state.done

		if state.err != nil {
			return nil, state.err
		}

		return state.value, nil
	}

	state := &buildState{
		done: make(chan struct{}),
	}

	r.building[name] = state

	r.mu.Unlock()

	nextStack := append(
		append([]string(nil), stack...),
		name,
	)

	value, err := provider.Build(&resolver{
		registry: r,
		stack:    nextStack,
	})

	if err != nil {
		if !errors.As(err, new(*Error)) {
			err = &Error{
				Kind:     ErrBuild,
				Provider: name,
				Cause:    err,
			}
		}

		r.mu.Lock()

		state.err = err
		delete(r.building, name)
		close(state.done)

		r.mu.Unlock()

		return nil, err
	}

	if provider.Lifetime != Singleton {
		err = fmt.Errorf(
			"provider %q uses unsupported lifetime %d",
			name,
			provider.Lifetime,
		)

		r.mu.Lock()

		state.err = err
		delete(r.building, name)
		close(state.done)

		r.mu.Unlock()

		return nil, err
	}

	r.mu.Lock()

	if r.closed {
		state.err = &Error{Kind: ErrClosed, Provider: name}
		delete(r.building, name)
		close(state.done)
		cleanup := provider.Cleanup
		r.mu.Unlock()
		if cleanup != nil {
			_ = cleanup(context.Background(), value)
		}
		return nil, state.err
	}

	state.value = value
	r.instances[name] = value

	if provider.Cleanup != nil {
		r.lifecycle.Add(name, func(ctx context.Context) error {
			return provider.Cleanup(ctx, value)
		})
	}

	delete(r.building, name)
	close(state.done)

	r.mu.Unlock()

	return value, nil
}

func (r *Registry) Close(ctx context.Context) error {
	r.mu.Lock()

	if r.closed {
		r.mu.Unlock()
		return nil
	}

	r.closed = true
	lifecycle := r.lifecycle

	r.mu.Unlock()

	return lifecycle.Close(ctx)
}
