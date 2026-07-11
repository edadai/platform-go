package runtime

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
)

type Component struct {
	Name string
	Run  func(context.Context) error
}

func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}

func Run(ctx context.Context, components ...Component) error {
	if len(components) == 0 {
		<-ctx.Done()
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(components))
	var wg sync.WaitGroup
	wg.Add(len(components))

	for _, component := range components {
		go func() {
			defer wg.Done()
			if component.Run == nil {
				return
			}
			if err := component.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("%s: %w", component.Name, err)
				cancel()
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		cancel()
		<-done
		return nil
	case err := <-errCh:
		cancel()
		<-done
		return err
	case <-done:
		return nil
	}
}
