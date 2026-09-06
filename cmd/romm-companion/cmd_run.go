package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/notify"
	"github.com/sdornan/romm-companion/internal/reconcile"
	"github.com/sdornan/romm-companion/internal/romm"
	"github.com/sdornan/romm-companion/internal/steam/paths"
	"github.com/sdornan/romm-companion/internal/steam/process"
)

func cmdRun(ctx context.Context, args []string) error {
	fs := newFlagSet("run")
	once := fs.Bool("once", false, "make one pass and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Paired() {
		return errors.New("not paired; run: romm-companion pair <server-url> <code>")
	}

	engine, err := newEngine(ctx, cfg)
	if err != nil {
		return err
	}

	if *once {
		report(engine.Run(ctx))
		return nil
	}

	// The server's event says only "look again", so it and the reconnect
	// callback feed one channel, and a burst collapses into a single pass.
	wake := make(chan struct{}, 1)
	nudge := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go engine.Client.WatchWithRetry(ctx, nudge, func(err error) {
		fmt.Fprintln(os.Stderr, "socket:", err)
	})

	fmt.Fprintf(os.Stderr, "watching %s\n", engine.Client.SocketURL())
	announce := announcer(ctx)
	err = engine.Loop(ctx, wake, func(result reconcile.Result, err error) {
		report(result, err)
		announce(result, err)
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func report(result reconcile.Result, err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "pass failed:", err)
		return
	}
	if result.Added > 0 || result.Removed > 0 || result.Staged > 0 || result.Failed > 0 {
		fmt.Fprintf(
			os.Stderr,
			"staged %d, added %d, removed %d, failed %d\n",
			result.Staged, result.Added, result.Removed, result.Failed,
		)
	}
	if result.Pending() {
		fmt.Fprintf(
			os.Stderr,
			"%d change(s) ready — they apply once Steam is closed\n", result.Deferred,
		)
	}
}

// newEngine wires the companion's pieces together and tells RomM what this PC
// can play, so the web UI can refuse an add before the user presses the button.
func newEngine(ctx context.Context, cfg *config.Config) (*reconcile.Engine, error) {
	install, err := paths.Find(cfg.SteamRoot)
	if err != nil {
		return nil, fmt.Errorf("%w (set steam_root in the config file to override)", err)
	}
	userDirs, err := install.UserDataDirs()
	if err != nil {
		return nil, err
	}
	userDir, err := pickUserDir(cfg, userDirs)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	coreMap, err := loadCoreMap(ctx, cfg)
	if err != nil {
		return nil, err
	}
	client := romm.New(cfg.ServerURL, cfg.Token, version)
	resolver := newResolver(cfg, coreMap)

	// Best-effort: an older server without the column still runs the queue.
	reportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.ReportCapabilities(
		reportCtx, cfg.DeviceID, resolver.Capabilities(platformSlugs(cfg, coreMap, resolver)),
	); err != nil {
		fmt.Fprintln(os.Stderr, "could not report playable platforms:", err)
	}

	return &reconcile.Engine{
		Client:       client,
		Cfg:          cfg,
		Resolver:     resolver,
		ExePath:      exe,
		UserDataDir:  userDir,
		SteamRunning: process.Running,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		},
	}, nil
}

// announcer returns a callback that raises a desktop notification when the
// state changes, and stays quiet otherwise. Without the transition check the
// "restart Steam" notice would fire on every re-check for as long as Steam
// stayed open.
func announcer(ctx context.Context) func(reconcile.Result, error) {
	wasPending := false
	return func(result reconcile.Result, err error) {
		if err != nil {
			return
		}
		if result.Added > 0 || result.Removed > 0 {
			notifyBest(ctx, "Steam library updated", libraryChangeText(result))
		}
		if result.Pending() && !wasPending {
			notifyBest(ctx, "Restart Steam to apply", pendingText(result.Deferred))
		}
		wasPending = result.Pending()
	}
}

func libraryChangeText(result reconcile.Result) string {
	switch {
	case result.Added > 0 && result.Removed > 0:
		return fmt.Sprintf("%s added, %s removed.",
			games(result.Added), games(result.Removed))
	case result.Added > 0:
		return fmt.Sprintf("%s added.", games(result.Added))
	default:
		return fmt.Sprintf("%s removed.", games(result.Removed))
	}
}

func pendingText(n int) string {
	if n == 1 {
		return "1 change is ready. It applies the next time Steam restarts."
	}
	return fmt.Sprintf(
		"%d changes are ready. They apply the next time Steam restarts.", n,
	)
}

func games(n int) string {
	if n == 1 {
		return "1 game"
	}
	return fmt.Sprintf("%d games", n)
}

// notifyBest reports a failed notification once, to stderr, and carries on: a
// headless or locked session is not an error worth stopping for.
func notifyBest(ctx context.Context, title, body string) {
	if err := notify.Send(ctx, title, body); err != nil {
		fmt.Fprintln(os.Stderr, "desktop notification unavailable:", err)
	}
}
