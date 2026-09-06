package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/emulator"
	"github.com/sdornan/romm-companion/internal/reconcile"
	"github.com/sdornan/romm-companion/internal/romm"
	"github.com/sdornan/romm-companion/internal/steam/paths"
	"github.com/sdornan/romm-companion/internal/steam/process"
)

// pollInterval is the backstop for the socket: the server nudges this device
// when its queue moves, but a missed event or a dropped connection must not
// strand a change, and a staged change also has to notice Steam closing.
const pollInterval = 30 * time.Second

func cmdRun(ctx context.Context, args []string) error {
	fs := newFlagSet("run")
	once := fs.Bool("once", false, "make one pass and exit")
	interval := fs.Duration("interval", pollInterval, "how often to re-check the queue")
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

	engine, err := newEngine(cfg)
	if err != nil {
		return err
	}

	if *once {
		return runPass(ctx, engine)
	}

	// The socket only ever says "look again", so it feeds the same channel the
	// ticker does and a burst collapses into one pass.
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

	fmt.Fprintf(os.Stderr, "watching %s (polling every %s)\n", cfg.ServerURL, *interval)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		if err := runPass(ctx, engine); err != nil {
			fmt.Fprintln(os.Stderr, "pass failed:", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-wake:
		case <-ticker.C:
		}
	}
}

func runPass(ctx context.Context, engine *reconcile.Engine) error {
	result, err := engine.Run(ctx)
	if err != nil {
		return err
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
			"%d change(s) ready — restart Steam to apply\n", result.Deferred,
		)
	}
	return nil
}

// newEngine wires the companion's pieces together and tells RomM what this PC
// can play, so the web UI can refuse an add before the user presses the button.
func newEngine(cfg *config.Config) (*reconcile.Engine, error) {
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

	client := romm.New(cfg.ServerURL, cfg.Token, version)
	resolver := emulator.NewResolver(cfg.Templates, coreMap)

	// Best-effort: an older server without the column still runs the queue.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.ReportCapabilities(
		ctx, cfg.DeviceID, resolver.Capabilities(platformSlugs(cfg)),
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
