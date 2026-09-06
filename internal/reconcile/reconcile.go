// Package reconcile applies RomM's shortcut queue to the local Steam library.
//
// The queue is a set of rows the server holds for this device. Each pass:
//
//	pending_add    → download the rom and its cover, then report `staged`
//	staged         → waiting for Steam to exit
//	pending_remove → waiting for Steam to exit
//
// Writes to shortcuts.vdf only happen while Steam is closed, because Steam
// reads that file at startup and rewrites it on exit, so a write underneath a
// running client is lost. Downloads are not gated that way, which is what lets
// the UI say "ready, restart Steam" instead of "queued" the whole time.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/emulator"
	"github.com/sdornan/romm-companion/internal/romm"
	"github.com/sdornan/romm-companion/internal/steam/artwork"
	"github.com/sdornan/romm-companion/internal/steam/paths"
	"github.com/sdornan/romm-companion/internal/steam/shortcuts"
)

// Shortcut queue statuses, mirroring the server's ShortcutStatus enum.
const (
	statusPendingAdd    = "pending_add"
	statusStaged        = "staged"
	statusPendingRemove = "pending_remove"
)

// Ack values the server accepts from a device.
const (
	ackStaged  = "staged"
	ackAdded   = "added"
	ackRemoved = "removed"
	ackFailed  = "failed"
)

// Engine applies one device's queue. Its collaborators are fields rather than
// package calls so a test can drive a full pass with no Steam and no network
// beyond a stub server.
type Engine struct {
	Client   *romm.Client
	Cfg      *config.Config
	Resolver *emulator.Resolver

	// ExePath is what the Steam shortcut runs. It is part of the app id Steam
	// derives, so moving the binary re-creates shortcuts under a new id.
	ExePath string
	// UserDataDir is the Steam userdata/<steamid> directory being written.
	UserDataDir string
	// SteamRunning gates every write. Defaults to the real process check.
	SteamRunning func() (bool, error)
	// Logf receives progress lines; nil discards them.
	Logf func(format string, args ...any)
}

// Result counts what one pass did, for the caller's log and the tray badge.
type Result struct {
	Staged   int
	Added    int
	Removed  int
	Failed   int
	Deferred int // ready on disk, waiting for Steam to close
}

// Pending reports whether anything is still waiting on a Steam restart.
func (r Result) Pending() bool { return r.Deferred > 0 }

func (e *Engine) logf(format string, args ...any) {
	if e.Logf != nil {
		e.Logf(format, args...)
	}
}

// Run performs one full pass over the queue.
func (e *Engine) Run(ctx context.Context) (Result, error) {
	var result Result

	rows, err := e.Client.PendingShortcuts(ctx)
	if err != nil {
		return result, fmt.Errorf("read queue: %w", err)
	}
	if len(rows) == 0 {
		return result, nil
	}

	// Stage first so the files are on disk before the write window opens.
	var ready []romm.Shortcut
	for _, row := range rows {
		switch row.Status {
		case statusPendingAdd:
			if err := e.stage(ctx, row); err != nil {
				result.Failed++
				e.logf("rom %d: %v", row.RomID, err)
				// A failed ack is not worth failing the pass over; the row
				// stays pending and the next pass tries again.
				if ackErr := e.Client.AckShortcut(
					ctx, row.ID, ackFailed, nil, err.Error(),
				); ackErr != nil {
					e.logf("rom %d: reporting failure: %v", row.RomID, ackErr)
				}
				continue
			}
			result.Staged++
			ready = append(ready, row)
		case statusStaged, statusPendingRemove:
			ready = append(ready, row)
		}
	}
	if len(ready) == 0 {
		return result, nil
	}

	running, err := e.steamRunning()
	if err != nil {
		return result, fmt.Errorf("check whether Steam is running: %w", err)
	}
	if running {
		result.Deferred = len(ready)
		e.logf("%d change(s) ready; waiting for Steam to close", len(ready))
		return result, nil
	}

	applied, err := e.apply(ctx, ready)
	result.Added += applied.Added
	result.Removed += applied.Removed
	result.Failed += applied.Failed
	return result, err
}

// stage downloads what the shortcut needs and reports it ready. Artwork is
// best-effort; a missing cover still leaves a working shortcut.
func (e *Engine) stage(ctx context.Context, row romm.Shortcut) error {
	rom, err := e.Client.GetRom(ctx, row.RomID)
	if err != nil {
		return fmt.Errorf("fetch rom: %w", err)
	}
	res := e.Resolver.Resolve(rom.PlatformSlug)
	if !res.Supported() {
		return fmt.Errorf("no emulator set up for platform %q", rom.PlatformSlug)
	}
	if _, err := e.download(ctx, rom); err != nil {
		return err
	}

	if cover := e.Client.DownloadCover(ctx, rom); cover != nil {
		appID := shortcuts.AppID(e.ExePath, rom.Title())
		if err := artwork.Write(
			paths.GridDir(e.UserDataDir), appID, artwork.Capsule, cover,
		); err != nil {
			e.logf("rom %d: cover art: %v", rom.ID, err)
		}
	}

	e.logf("staged %s", rom.Title())
	return e.Client.AckShortcut(ctx, row.ID, ackStaged, nil, "")
}

// download fetches every game file into <download dir>/<platform>/ and returns
// the one to hand the emulator: the playlist or cue sheet for a multi-file
// game, otherwise the single file.
func (e *Engine) download(ctx context.Context, rom *romm.Rom) (string, error) {
	dir := filepath.Join(e.Cfg.DownloadDir, rom.PlatformSlug)
	var entry string
	for _, f := range rom.Files {
		if f.Category != "" && f.Category != "game" {
			continue
		}
		path, err := e.Client.DownloadFile(ctx, f, dir)
		if err != nil {
			return "", fmt.Errorf("download %s: %w", f.FileName, err)
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".m3u":
			entry = path
		case ".cue":
			if entry == "" || !strings.EqualFold(filepath.Ext(entry), ".m3u") {
				entry = path
			}
		default:
			if entry == "" {
				entry = path
			}
		}
	}
	if entry == "" {
		return "", errors.New("rom has no downloadable game files")
	}
	return entry, nil
}

// apply opens shortcuts.vdf once, makes every queued change, writes it back,
// and only then reports each row. Acking after the write means a crash
// mid-pass leaves rows queued rather than claiming a shortcut that is not
// there.
func (e *Engine) apply(ctx context.Context, rows []romm.Shortcut) (Result, error) {
	var result Result

	file, err := e.readShortcuts()
	if err != nil {
		return result, err
	}

	type change struct {
		row    romm.Shortcut
		appID  uint32
		remove bool
	}
	var changes []change

	for _, row := range rows {
		if row.Status == statusPendingRemove {
			// Read the entry before dropping it: its app id names the artwork
			// files that go with it.
			existing, found := file.Lookup(row.RomID)
			if !found {
				// Already gone: still ack so the server can drop the row.
				e.logf("rom %d: no shortcut to remove", row.RomID)
			}
			file.Remove(row.RomID)
			if e.Cfg.DeleteOnRemove {
				e.deleteDownload(ctx, row.RomID)
			}
			changes = append(changes, change{
				row: row, appID: existing.AppID, remove: true,
			})
			continue
		}

		rom, err := e.Client.GetRom(ctx, row.RomID)
		if err != nil {
			result.Failed++
			e.logf("rom %d: %v", row.RomID, err)
			continue
		}
		// Re-download rather than trust the staging pass: a row can reach
		// `staged` in an earlier run whose files were since deleted.
		if _, err := e.download(ctx, rom); err != nil {
			result.Failed++
			e.logf("rom %d: %v", row.RomID, err)
			continue
		}
		appID := file.Upsert(shortcuts.Shortcut{
			RomID:         rom.ID,
			AppName:       rom.Title(),
			Exe:           e.ExePath,
			StartDir:      filepath.Dir(e.ExePath),
			LaunchOptions: fmt.Sprintf("launch --rom %d", rom.ID),
			Icon:          e.iconPath(rom),
		})
		changes = append(changes, change{row: row, appID: appID})
	}

	if len(changes) == 0 {
		return result, nil
	}
	if err := e.writeShortcuts(file); err != nil {
		return result, err
	}

	for _, c := range changes {
		if c.remove {
			if c.appID != 0 {
				if err := artwork.Remove(paths.GridDir(e.UserDataDir), c.appID); err != nil {
					e.logf("rom %d: clearing art: %v", c.row.RomID, err)
				}
			}
			if err := e.Client.AckShortcut(ctx, c.row.ID, ackRemoved, nil, ""); err != nil {
				e.logf("rom %d: reporting removal: %v", c.row.RomID, err)
				continue
			}
			result.Removed++
			continue
		}
		appID := c.appID
		if err := e.Client.AckShortcut(ctx, c.row.ID, ackAdded, &appID, ""); err != nil {
			e.logf("rom %d: reporting add: %v", c.row.RomID, err)
			continue
		}
		result.Added++
	}
	e.logf("wrote %d change(s) to Steam", len(changes))
	return result, nil
}

// deleteDownload removes the rom's files from the download directory. It is
// best-effort: the rom may be gone from the server, and a file another
// frontend still uses is not worth failing a removal over.
func (e *Engine) deleteDownload(ctx context.Context, romID int) {
	rom, err := e.Client.GetRom(ctx, romID)
	if err != nil {
		e.logf("rom %d: cannot resolve files to delete: %v", romID, err)
		return
	}
	dir := filepath.Join(e.Cfg.DownloadDir, rom.PlatformSlug)
	for _, f := range rom.Files {
		path := filepath.Join(dir, filepath.Base(f.FileName))
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			e.logf("rom %d: deleting %s: %v", romID, path, err)
		}
	}
}

// iconPath points Steam at the cover already on disk, when there is one.
func (e *Engine) iconPath(rom *romm.Rom) string {
	if rom.PathCoverL == "" {
		return ""
	}
	name := artwork.FileName(shortcuts.AppID(e.ExePath, rom.Title()), artwork.Capsule)
	return filepath.Join(paths.GridDir(e.UserDataDir), name)
}

func (e *Engine) readShortcuts() (*shortcuts.File, error) {
	path := paths.ShortcutsFile(e.UserDataDir)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		// A Steam install with no non-Steam games yet.
		return shortcuts.New(), nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return shortcuts.Read(f)
}

// writeShortcuts replaces the file atomically so an interrupted write cannot
// leave Steam with a truncated library.
func (e *Engine) writeShortcuts(file *shortcuts.File) error {
	path := paths.ShortcutsFile(e.UserDataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := file.Write(out); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (e *Engine) steamRunning() (bool, error) {
	if e.SteamRunning == nil {
		return false, errors.New("reconcile: no Steam process check configured")
	}
	return e.SteamRunning()
}
