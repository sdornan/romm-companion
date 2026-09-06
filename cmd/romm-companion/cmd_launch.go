package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/cores"
	"github.com/sdornan/romm-companion/internal/emulator"
	"github.com/sdornan/romm-companion/internal/launcher"
	"github.com/sdornan/romm-companion/internal/romm"
)

// loadCoreMap fetches RomM's core map, warning and carrying on when only the
// cached copy is available so a launch still works offline.
func loadCoreMap(ctx context.Context, cfg *config.Config) (cores.Map, error) {
	if cfg.ServerURL == "" {
		return nil, errNotPaired
	}
	client := romm.New(cfg.ServerURL, cfg.Token, version)
	m, err := cores.Load(ctx, client)
	if err != nil && m == nil {
		return nil, fmt.Errorf("could not load the emulator core map: %w", err)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "using the cached emulator core map:", err)
	}
	return m, nil
}

// newResolver builds the platform-to-emulator resolver for this machine.
func newResolver(cfg *config.Config, coreMap cores.Map) *emulator.Resolver {
	r := emulator.NewResolver(cfg.Templates, coreMap)
	r.RomRoot = cfg.DownloadDir
	return r
}

// platformSlugs is every platform this build knows how to resolve: RomM's core
// map, ES-DE's standalone emulators, and whatever the user wrote a template for.
func platformSlugs(cfg *config.Config, coreMap cores.Map, r *emulator.Resolver) []string {
	var slugs []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			slugs, seen[s] = append(slugs, s), true
		}
	}
	for s := range coreMap {
		add(s)
	}
	if r.ESDE != nil {
		for s := range r.ESDE.Systems {
			add(s)
		}
	}
	for s := range cfg.Templates {
		add(s)
	}
	sort.Strings(slugs)
	return slugs
}

func cmdCapabilities(ctx context.Context, args []string) error {
	fs := newFlagSet("capabilities")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	coreMap, err := loadCoreMap(ctx, cfg)
	if err != nil {
		return err
	}
	r := newResolver(cfg, coreMap)
	slugs := platformSlugs(cfg, coreMap, r)
	if r.RetroArch == "" {
		fmt.Println("RetroArch: not found on PATH")
	} else {
		fmt.Println("RetroArch:", r.RetroArch)
	}
	for _, s := range slugs {
		res := r.Resolve(s)
		if res.Supported() {
			fmt.Printf("  %-26s %s\n", s, res.Descriptor)
		} else {
			fmt.Printf("  %-26s (none)\n", s)
		}
	}
	return nil
}

func cmdLaunch(ctx context.Context, args []string) error {
	fs := newFlagSet("launch")
	romID := fs.Int("rom", 0, "RomM rom id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *romID <= 0 {
		return errors.New("usage: launch --rom <id>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Paired() {
		return errNotPaired
	}
	client := romm.New(cfg.ServerURL, cfg.Token, version)

	rom, err := client.GetRom(ctx, *romID)
	if err != nil {
		return err
	}
	coreMap, err := loadCoreMap(ctx, cfg)
	if err != nil {
		return err
	}
	res := newResolver(cfg, coreMap).Resolve(rom.PlatformSlug)
	if !res.Supported() || res.Kind == "web_player" {
		return fmt.Errorf("no emulator set up for platform %q on this PC", rom.PlatformSlug)
	}

	romPath, err := ensureDownloaded(ctx, client, rom, cfg.DownloadDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Launching %s with %s\n", rom.Name, res.Descriptor)
	session := launcher.Run(ctx, res, romPath)
	if session.Err != nil {
		fmt.Fprintln(os.Stderr, "emulator exited with error:", session.Err)
	}
	if session.Duration() > 0 {
		ps := romm.PlaySession{RomID: rom.ID, StartTime: session.Start, EndTime: session.End, DurationMS: session.Duration().Milliseconds()}
		if err := client.IngestPlaySessions(ctx, cfg.DeviceID, []romm.PlaySession{ps}); err != nil {
			fmt.Fprintln(os.Stderr, "could not record play session:", err)
		}
	}
	return nil
}

// ensureDownloaded fetches the ROM's files into <download_dir>/<platform>/ and
// returns the file to hand to the emulator: the playlist or cue sheet for a
// multi-file game, otherwise the single file.
func ensureDownloaded(ctx context.Context, client *romm.Client, rom *romm.Rom, downloadDir string) (string, error) {
	dir := filepath.Join(downloadDir, rom.PlatformSlug)
	var entry string
	for _, f := range rom.Files {
		if f.Category != "" && f.Category != "game" {
			continue
		}
		p, err := client.DownloadFile(ctx, f, dir)
		if err != nil {
			return "", fmt.Errorf("download %s: %w", f.FileName, err)
		}
		switch ext := filepath.Ext(p); ext {
		case ".m3u":
			entry = p
		case ".cue":
			if entry == "" || filepath.Ext(entry) != ".m3u" {
				entry = p
			}
		default:
			if entry == "" {
				entry = p
			}
		}
	}
	if entry == "" {
		return "", errors.New("rom has no downloadable game files")
	}
	return entry, nil
}
