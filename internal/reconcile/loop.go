package reconcile

import (
	"context"
	"time"
)

// SteamWatchInterval is how often the local Steam process is re-checked while a
// change is waiting to be written.
//
// This is a local read (of /proc, or the OS process list), not a request to
// RomM, and it only runs while something is actually staged. No operating
// system offers a portable "that process exited" notification for a process
// this program did not start, so a short local check is how the write window
// is noticed.
const SteamWatchInterval = 5 * time.Second

// Loop applies the queue whenever wake fires and, while a change is waiting on
// Steam, re-checks the local Steam process until the write window opens.
//
// Nothing here polls RomM. `wake` is driven by the server's `shortcuts:changed`
// event, plus one fire per (re)connection so a queue that moved while the
// socket was down is still picked up.
//
// onResult, when set, receives the outcome of every pass, including failures,
// so the caller can report progress without Loop knowing how.
func (e *Engine) Loop(
	ctx context.Context,
	wake <-chan struct{},
	onResult func(Result, error),
) error {
	for {
		result, err := e.pass(ctx)
		if onResult != nil {
			onResult(result, err)
		}

		if err := e.waitForWork(ctx, wake, result.Pending()); err != nil {
			return err
		}
	}
}

// waitForWork blocks until the server nudges this device or, when a change is
// staged, until it is time to re-check whether Steam has closed.
func (e *Engine) waitForWork(
	ctx context.Context,
	wake <-chan struct{},
	watchSteam bool,
) error {
	var steamCheck <-chan time.Time
	if watchSteam {
		ticker := time.NewTicker(SteamWatchInterval)
		defer ticker.Stop()
		steamCheck = ticker.C
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wake:
		return nil
	case <-steamCheck:
		return nil
	}
}
