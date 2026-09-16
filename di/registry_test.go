package di_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edadai/platform-go/di"
)

func TestRegistry_ProvideAndResolve(t *testing.T) {
	r := di.NewRegistry()

	err := r.Provide(di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return "hello", nil
		},
	})
	if err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	value, err := r.Resolve("config")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got, want := value.(string), "hello"; got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestRegistry_DuplicateProvider(t *testing.T) {
	r := di.NewRegistry()

	provider := di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return "value", nil
		},
	}

	if err := r.Provide(provider); err != nil {
		t.Fatalf("first Provide() error = %v", err)
	}

	err := r.Provide(provider)
	if err == nil {
		t.Fatal("second Provide() expected error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrDuplicate {
		t.Fatalf("error kind = %q, want %q", diErr.Kind, di.ErrDuplicate)
	}

	if diErr.Provider != "config" {
		t.Fatalf("error provider = %q, want %q", diErr.Provider, "config")
	}
}

func TestRegistry_ProviderValidation(t *testing.T) {
	tests := []struct {
		name     string
		provider di.Provider
		wantText string
	}{
		{
			name: "empty name",
			provider: di.Provider{
				Build: func(di.Resolver) (any, error) {
					return nil, nil
				},
			},
			wantText: "provider name cannot be empty",
		},
		{
			name: "nil builder",
			provider: di.Provider{
				Name: "config",
			},
			wantText: `provider "config" has no builder`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := di.NewRegistry()

			err := r.Provide(tt.provider)
			if err == nil {
				t.Fatal("Provide() expected error, got nil")
			}

			if err.Error() != tt.wantText {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantText)
			}
		})
	}
}

func TestRegistry_ProviderNotFound(t *testing.T) {
	r := di.NewRegistry()

	_, err := r.Resolve("database")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrProviderNotFound {
		t.Fatalf(
			"error kind = %q, want %q",
			diErr.Kind,
			di.ErrProviderNotFound,
		)
	}

	if diErr.Provider != "database" {
		t.Fatalf("provider = %q, want %q", diErr.Provider, "database")
	}

	want := `dependency "database" not registered`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestRegistry_Singleton(t *testing.T) {
	r := di.NewRegistry()

	var builds atomic.Int32

	if err := r.Provide(di.Provider{
		Name: "counter",
		Build: func(di.Resolver) (any, error) {
			builds.Add(1)

			return &struct {
				Value int
			}{Value: 42}, nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	first, err := r.Resolve("counter")
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}

	second, err := r.Resolve("counter")
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}

	if first != second {
		t.Fatal("singleton provider returned different instances")
	}

	if got := builds.Load(); got != 1 {
		t.Fatalf("builder called %d times, want 1", got)
	}
}

func TestRegistry_DependencyResolution(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return "production", nil
		},
	}); err != nil {
		t.Fatalf("Provide(config) error = %v", err)
	}

	if err := r.Provide(di.Provider{
		Name: "database",
		Build: func(resolve di.Resolver) (any, error) {
			config, err := di.As[string](resolve, "config")
			if err != nil {
				return nil, err
			}

			return "db:" + config, nil
		},
	}); err != nil {
		t.Fatalf("Provide(database) error = %v", err)
	}

	value, err := r.Resolve("database")
	if err != nil {
		t.Fatalf("Resolve(database) error = %v", err)
	}

	if got, want := value.(string), "db:production"; got != want {
		t.Fatalf("Resolve(database) = %q, want %q", got, want)
	}
}

func TestRegistry_BuildError(t *testing.T) {
	r := di.NewRegistry()

	buildErr := errors.New("connection refused")

	if err := r.Provide(di.Provider{
		Name: "database",
		Build: func(di.Resolver) (any, error) {
			return nil, buildErr
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	_, err := r.Resolve("database")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrBuild {
		t.Fatalf("error kind = %q, want %q", diErr.Kind, di.ErrBuild)
	}

	if diErr.Provider != "database" {
		t.Fatalf("provider = %q, want %q", diErr.Provider, "database")
	}

	if !errors.Is(err, buildErr) {
		t.Fatal("wrapped build error is not discoverable with errors.Is")
	}
}

func TestRegistry_MissingDependency(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "service",
		Build: func(resolve di.Resolver) (any, error) {
			return resolve.Resolve("database")
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	_, err := r.Resolve("service")
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}

	if !strings.Contains(err.Error(), `dependency "database" not registered`) {
		t.Fatalf("error = %q, expected missing dependency", err)
	}
}

func TestRegistry_CircularDependency(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "a",
		Build: func(resolve di.Resolver) (any, error) {
			return resolve.Resolve("b")
		},
	}); err != nil {
		t.Fatalf("Provide(a) error = %v", err)
	}

	if err := r.Provide(di.Provider{
		Name: "b",
		Build: func(resolve di.Resolver) (any, error) {
			return resolve.Resolve("a")
		},
	}); err != nil {
		t.Fatalf("Provide(b) error = %v", err)
	}

	_, err := r.Resolve("a")
	if err == nil {
		t.Fatal("Resolve() expected circular dependency error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrCircular {
		t.Fatalf(
			"error kind = %q, want %q",
			diErr.Kind,
			di.ErrCircular,
		)
	}

	if diErr.Provider != "a" {
		t.Fatalf("provider = %q, want %q", diErr.Provider, "a")
	}
}

func TestRegistry_Cleanup(t *testing.T) {
	r := di.NewRegistry()

	var mu sync.Mutex
	var cleanupOrder []string

	addCleanup := func(name string) func(context.Context, any) error {
		return func(context.Context, any) error {
			mu.Lock()
			defer mu.Unlock()

			cleanupOrder = append(cleanupOrder, name)
			return nil
		}
	}

	if err := r.Provide(di.Provider{
		Name: "database",
		Build: func(di.Resolver) (any, error) {
			return "database", nil
		},
		Cleanup: addCleanup("database"),
	}); err != nil {
		t.Fatalf("Provide(database) error = %v", err)
	}

	if err := r.Provide(di.Provider{
		Name: "service",
		Build: func(resolve di.Resolver) (any, error) {
			db, err := di.As[string](resolve, "database")
			if err != nil {
				return nil, err
			}

			return "service using " + db, nil
		},
		Cleanup: addCleanup("service"),
	}); err != nil {
		t.Fatalf("Provide(service) error = %v", err)
	}

	if _, err := r.Resolve("service"); err != nil {
		t.Fatalf("Resolve(service) error = %v", err)
	}

	if err := r.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	want := []string{"service", "database"}
	if !reflect.DeepEqual(cleanupOrder, want) {
		t.Fatalf("cleanup order = %v, want %v", cleanupOrder, want)
	}
}

func TestRegistry_ResolveAfterClose(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return "value", nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	if err := r.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err := r.Resolve("config")
	if err == nil {
		t.Fatal("Resolve() after Close() expected error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrClosed {
		t.Fatalf("error kind = %q, want %q", diErr.Kind, di.ErrClosed)
	}

	err = r.Provide(di.Provider{
		Name: "other",
		Build: func(di.Resolver) (any, error) {
			return "other", nil
		},
	})
	if err == nil {
		t.Fatal("Provide() after Close() expected error, got nil")
	}

	if !errors.As(err, &diErr) || diErr.Kind != di.ErrClosed {
		t.Fatalf("Provide() after Close() error = %v, want ErrClosed", err)
	}
}

func TestAs_TypeMismatch(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return 42, nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	_, err := di.As[string](r, "config")
	if err == nil {
		t.Fatal("As[string] expected error, got nil")
	}

	var diErr *di.Error
	if !errors.As(err, &diErr) {
		t.Fatalf("error type = %T, want *di.Error", err)
	}

	if diErr.Kind != di.ErrTypeMismatch {
		t.Fatalf("error kind = %q, want %q", diErr.Kind, di.ErrTypeMismatch)
	}
}

func TestAs_Success(t *testing.T) {
	r := di.NewRegistry()

	if err := r.Provide(di.Provider{
		Name: "config",
		Build: func(di.Resolver) (any, error) {
			return "production", nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	got, err := di.As[string](r, "config")
	if err != nil {
		t.Fatalf("As[string] error = %v", err)
	}

	if got != "production" {
		t.Fatalf("As[string] = %q, want production", got)
	}
}

func TestLifecycle_CleanupReverseOrder(t *testing.T) {
	var lifecycle di.Lifecycle

	var mu sync.Mutex
	var got []string

	lifecycle.Add("first", func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()

		got = append(got, "first")
		return nil
	})

	lifecycle.Add("second", func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()

		got = append(got, "second")
		return nil
	})

	lifecycle.Add("third", func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()

		got = append(got, "third")
		return nil
	})

	if err := lifecycle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	want := []string{"third", "second", "first"}

	if len(got) != len(want) {
		t.Fatalf("cleanup count = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cleanup order = %v, want %v", got, want)
		}
	}
}

func TestLifecycle_CleanupErrors(t *testing.T) {
	var lifecycle di.Lifecycle

	errFirst := errors.New("first cleanup failed")
	errSecond := errors.New("second cleanup failed")

	lifecycle.Add("first", func(context.Context) error {
		return errFirst
	})

	lifecycle.Add("second", func(context.Context) error {
		return errSecond
	})

	err := lifecycle.Close(context.Background())
	if err == nil {
		t.Fatal("Close() expected error, got nil")
	}

	if !errors.Is(err, errFirst) {
		t.Fatal("Close() error does not contain first cleanup error")
	}

	if !errors.Is(err, errSecond) {
		t.Fatal("Close() error does not contain second cleanup error")
	}

	var diErrs []*di.Error

	var walk func(error)

	walk = func(err error) {
		if err == nil {
			return
		}

		var dErr *di.Error
		if errors.As(err, &dErr) {
			diErrs = append(diErrs, dErr)
		}

		type unwrapMany interface {
			Unwrap() []error
		}

		if many, ok := err.(unwrapMany); ok {
			for _, child := range many.Unwrap() {
				walk(child)
			}
		}
	}

	walk(err)

	if len(diErrs) < 2 {
		t.Fatalf("found %d *di.Error values, want at least 2", len(diErrs))
	}

	for _, diErr := range diErrs {
		if diErr.Kind != di.ErrCleanup {
			t.Fatalf("cleanup error kind = %q, want %q", diErr.Kind, di.ErrCleanup)
		}
	}
}

func TestLifecycle_NilCleanupIgnored(t *testing.T) {
	var lifecycle di.Lifecycle

	lifecycle.Add("nil", nil)

	if err := lifecycle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestRegistry_ConcurrentResolve(t *testing.T) {
	r := di.NewRegistry()

	var builds atomic.Int32

	if err := r.Provide(di.Provider{
		Name: "singleton",
		Build: func(di.Resolver) (any, error) {
			builds.Add(1)

			return &struct{}{}, nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	const goroutines = 100

	results := make(chan any, goroutines)
	errs := make(chan error, goroutines)

	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			value, err := r.Resolve("singleton")
			if err != nil {
				errs <- err
				return
			}

			results <- value
		}()
	}

	wg.Wait()

	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("Resolve() error = %v", err)
	}

	var first any
	count := 0

	for value := range results {
		if count == 0 {
			first = value
		} else if value != first {
			t.Fatal("concurrent Resolve() returned different singleton instances")
		}

		count++
	}

	if count != goroutines {
		t.Fatalf("got %d results, want %d", count, goroutines)
	}

	if got := builds.Load(); got != 1 {
		t.Fatalf("builder was called %d time(s), want 1", got)
	}
}

func TestRegistry_SingletonBuildsOnceConcurrently(t *testing.T) {
	r := di.NewRegistry()

	var builds atomic.Int32

	if err := r.Provide(di.Provider{
		Name: "service",
		Build: func(di.Resolver) (any, error) {
			builds.Add(1)

			time.Sleep(50 * time.Millisecond)

			return &struct{}{}, nil
		},
	}); err != nil {
		t.Fatalf("Provide() error = %v", err)
	}

	const goroutines = 100

	results := make(chan any, goroutines)
	errs := make(chan error, goroutines)

	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			value, err := r.Resolve("service")
			if err != nil {
				errs <- err
				return
			}

			results <- value
		}()
	}

	wg.Wait()

	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("Resolve() error = %v", err)
	}

	var first any

	for value := range results {
		if first == nil {
			first = value
			continue
		}

		if value != first {
			t.Fatal("different singleton instances returned")
		}
	}

	if got := builds.Load(); got != 1 {
		t.Fatalf("Build() called %d times, want 1", got)
	}
}
