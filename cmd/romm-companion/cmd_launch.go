package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/emulator"
	"github.com/sdornan/romm-companion/internal/launcher"
	"github.com/sdornan/romm-companion/internal/romm"
)

// coreMap is RomM's platform-to-libretro-core map. Until RomM exposes it over
// the API (proposed GET /api/config/emulator-cores) a small seed keeps the
// common platforms working; the server copy replaces this once it exists.
var coreMap = map[string][]string{
	"nes": {"fceumm", "nestopia"}, "snes": {"snes9x", "bsnes"}, "n64": {"mupen64plus_next", "parallel_n64"},
	"gb": {"gambatte"}, "gbc": {"gambatte"}, "gba": {"mgba"}, "nds": {"melonds", "desmume"},
	"genesis-slash-megadrive": {"genesis_plus_gx", "picodrive"}, "sms": {"genesis_plus_gx"}, "gamegear": {"genesis_plus_gx"},
	"segacd": {"genesis_plus_gx"}, "sega32": {"picodrive"}, "saturn": {"yabause"},
	"psx": {"pcsx_rearmed", "mednafen_psx_hw"}, "psp": {"ppsspp"},
	"arcade": {"fbneo", "mame2003_plus"}, "neogeo": {"fbneo"},
	"atari2600": {"stella2014"}, "atari7800": {"prosystem"}, "lynx": {"handy"}, "jaguar": {"virtualjaguar"},
	"pce": {"mednafen_pce"}, "ngp": {"mednafen_ngp"}, "wonderswan": {"mednafen_wswan"},
	"vb": {"beetle_vb"}, "3do": {"opera"}, "dos": {"dosbox_pure"},
}

func cmdCapabilities(_ context.Context, args []string) error {
	fs := newFlagSet("capabilities")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	r := emulator.NewResolver(cfg.Templates, coreMap)
	slugs := make([]string, 0, len(coreMap)+len(cfg.Templates))
	seen := map[string]bool{}
	for s := range coreMap {
		if !seen[s] {
			slugs, seen[s] = append(slugs, s), true
		}
	}
	for s := range cfg.Templates {
		if !seen[s] {
			slugs, seen[s] = append(slugs, s), true
		}
	}
	sort.Strings(slugs)
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
		return errors.New("not paired; run: romm-companion pair <server-url> <code>")
	}
	client := romm.New(cfg.ServerURL, cfg.Token, version)

	rom, err := client.GetRom(ctx, *romID)
	if err != nil {
		return err
	}
	res := emulator.NewResolver(cfg.Templates, coreMap).Resolve(rom.PlatformSlug)
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
