package reconcile

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// The loop's timing is the point of these tests, so they run under synctest:
// its clock advances only when every goroutine is blocked, which makes "waited
// exactly one interval" an assertion rather than a race.

func TestLoopDoesNotRunAgainUntilWokenWhenNothingIsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var passes atomic.Int32
		engine := loopEngine(&passes, Result{})
		wake := make(chan struct{}, 1)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = engine.Loop(ctx, wake, nil) }()

		synctest.Wait()
		if got := passes.Load(); got != 1 {
			t.Fatalf("one pass on entry, got %d", got)
		}

		// A whole day of no events must not produce a second pass: with
		// nothing staged there is no local process to watch and no reason to
		// ask the server anything.
		time.Sleep(24 * time.Hour)
		synctest.Wait()
		if got := passes.Load(); got != 1 {
			t.Fatalf("idle loop ran %d passes; it should wait for the socket", got)
		}

		wake <- struct{}{}
		synctest.Wait()
		if got := passes.Load(); got != 2 {
			t.Fatalf("a nudge should run one more pass, got %d", got)
		}
	})
}

func TestLoopRechecksSteamWhileAChangeIsStaged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var passes atomic.Int32
		engine := loopEngine(&passes, Result{Deferred: 1})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = engine.Loop(ctx, make(chan struct{}), nil) }()

		synctest.Wait()
		if got := passes.Load(); got != 1 {
			t.Fatalf("passes = %d", got)
		}
		// Steam is still up, so the loop re-checks on its own.
		time.Sleep(SteamWatchInterval)
		synctest.Wait()
		if got := passes.Load(); got != 2 {
			t.Fatalf("want a re-check after one interval, got %d passes", got)
		}
	})
}

func TestLoopReportsFailuresAndKeepsGoing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var passes atomic.Int32
		engine := loopEngine(&passes, Result{})
		engine.runFn = func(context.Context) (Result, error) {
			passes.Add(1)
			return Result{}, errors.New("server down")
		}

		var seen atomic.Int32
		wake := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			_ = engine.Loop(ctx, wake, func(_ Result, err error) {
				if err != nil {
					seen.Add(1)
				}
			})
		}()

		synctest.Wait()
		wake <- struct{}{}
		synctest.Wait()
		if seen.Load() != 2 || passes.Load() != 2 {
			t.Fatalf("a failed pass must be reported and not stop the loop: %d/%d",
				seen.Load(), passes.Load())
		}
	})
}

func TestLoopStopsWithItsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var passes atomic.Int32
		engine := loopEngine(&passes, Result{})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- engine.Loop(ctx, make(chan struct{}), nil) }()

		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Loop should return its context's error, got %v", err)
		}
	})
}

// loopEngine returns an Engine whose pass is stubbed, so the tests above
// exercise the waiting rather than the queue.
func loopEngine(passes *atomic.Int32, result Result) *Engine {
	return &Engine{
		runFn: func(context.Context) (Result, error) {
			passes.Add(1)
			return result, nil
		},
	}
}
