# RomM Companion

A small desktop agent that connects a gaming PC to a [RomM](https://github.com/rommapp/romm) server. Press **Add to Steam** on a game in RomM and it appears in your Steam library on this PC, with artwork, launching through a local emulator. Playtime and saves report back to RomM.

Launching from RomM without Steam ("Play on Desktop") is a later phase; see the design.

It runs on Windows, Linux and macOS as a single binary. Full design in [docs/DESIGN.md](docs/DESIGN.md).

## Status

Pre-alpha. The pieces that only depend on the local machine and RomM's existing API work today:

| Command | State |
| --- | --- |
| `pair <server> <code>` | Works against RomM's client token pairing and device registration. |
| `steam paths` | Finds the Steam install and the active `userdata/<id>` directory. |
| `steam list` | Reads `shortcuts.vdf` and lists the entries this tool owns. |
| `capabilities` | Shows which emulator would open each platform on this PC. |
| `launch --rom <id>` | Downloads the ROM, runs the resolved emulator, records the play session. |
| `run` | Watches the queue and applies it: downloads, artwork, and the `shortcuts.vdf` write. |
| `service install` | Registers `run` to start at login. `uninstall` and `status` go with it. |

The server side (shortcut table, routes, socket event, capability column) is on a RomM branch and is not merged yet, so `run` has nothing to talk to against a stock RomM.

## Install

Grab the archive for your OS from [Releases](https://github.com/sdornan/romm-companion/releases), unpack it, and put `romm-companion` on your PATH. Every tagged release ships Linux, macOS and Windows builds for amd64 and arm64, plus a `checksums.txt` to verify a download against.

The binaries are unsigned, so macOS Gatekeeper and Windows SmartScreen warn on first run. On macOS, clear the quarantine flag with `xattr -d com.apple.quarantine romm-companion`.

## How `run` works

`run` reports which platforms this PC can play, then waits for work. Nothing polls RomM: the server emits `shortcuts:changed` over Socket.IO when the queue moves, and each (re)connection re-reads the queue so a change made while the socket was down is still picked up.

Each pass:

1. **Stage.** For every `pending_add` row: download the game's files and its artwork, and report `staged`. RomM's cover becomes Steam's vertical capsule; RomM also fronts SteamGridDB for the hero and logo on the game's page, so no API key lives here. A platform with no emulator on this PC fails that row alone, with the reason, and the rest of the pass continues.
2. **Apply.** If Steam is **not** running, open `shortcuts.vdf`, add the staged games and drop the removed ones, write it back atomically, then report `added` or `removed`. Shortcuts this tool did not create are never touched.
3. **Defer.** If Steam **is** running, nothing is written: Steam reads that file at startup and rewrites it on exit, so a write underneath it is lost. The change stays staged, and `run` re-checks the local Steam process every few seconds until the window opens. That check is a local process read, not a request to RomM, and it only runs while something is actually staged.

Reporting happens after the write, so an interrupted pass leaves rows queued rather than claiming a shortcut that is not there.

## How a platform picks an emulator

Four layers, first match wins:

1. **Your template.** A command in the config file under `templates`, keyed by RomM platform slug.
2. **A standalone emulator.** ES-DE's system definitions list the alternatives per platform in preference order; the first whose emulator is actually installed here wins. ES-DE's find rules say where to look: names on your PATH, then the usual install locations per OS.
3. **RetroArch.** With the core RomM's own map names for the platform.
4. **The RomM web player**, when a URL is configured.

`capabilities` prints the outcome for every platform. The ES-DE tables are generated from a pinned release into `internal/emulator/esde/`; to refresh them, run `go run ./tools/gen-esde -ref vX.Y.Z` and commit the result. ES-DE is MIT licensed; the notice travels with the data in `internal/emulator/esde/LICENSE.ES-DE`.

`tools/gen-esde/romm_slugs.json` joins ES-DE's system names to RomM slugs. Regenerate it from a RomM checkout:

```sh
python3 -c "
import json
from utils.platform_aliases import PLATFORM_FS_ALIASES
from utils.platform_slugs import UniversalPlatformSlug as UPS
json.dump({'aliases': {k: str(v) for k, v in PLATFORM_FS_ALIASES.items()},
           'slugs': sorted(str(s) for s in UPS)}, open('romm_slugs.json', 'w'), indent=2, sort_keys=True)
"
```

## Running it in the background

`romm-companion service install` registers `run` to start at login, using each platform's own mechanism: a systemd user unit on Linux, a launchd agent on macOS, a per-user registry Run entry on Windows. `service status` says where that entry lives and `service uninstall` removes it.

There is no tray icon. A tray needs cgo on Linux and macOS, which would cost the single static binary and the clean cross-compile, so the companion talks through the desktop's own notifications instead: one when a change is ready and Steam needs restarting, one when the library actually changes. Notifications are best-effort, and a headless or locked session never fails a pass.

## Build

Go 1.25 or newer.

```sh
go build ./cmd/romm-companion
go test ./...
```

Cross-compile with `GOOS`/`GOARCH` as usual; there are no cgo dependencies.

To cut a release, push a `v*` tag: GoReleaser builds and publishes the archives. Check the pipeline without tagging with `goreleaser release --snapshot --clean`.

## Try it

1. In RomM, open Settings, create a client token with the `devices.read`, `devices.write`, `roms.read`, `roms.user.read`, `roms.user.write` and `assets.read` scopes, and click **Pair** to get a code.
2. `romm-companion pair https://romm.example ABC-123`
3. `romm-companion capabilities` to see what this PC can play. Add overrides in the config file under `templates`, keyed by RomM platform slug, using `%ROM%` for the file path:

   ```json
   { "templates": { "ps2": "pcsx2-qt -batch \"%ROM%\"" } }
   ```

4. `romm-companion run` and press **Add to Steam** on a game in RomM. Restart Steam when it says a change is ready.

Or launch without Steam: `romm-companion launch --rom 123`.

Settings live at `$ROMM_COMPANION_CONFIG` or `<OS config dir>/romm-companion/config.json`.

## Layout

```
cmd/romm-companion/      CLI entry point and commands
internal/steam/vdf/      Binary KeyValues reader and writer
internal/steam/shortcuts/ shortcuts.vdf entries, app id derivation, RomM ownership tags
internal/steam/paths/    Steam install and userdata discovery per OS
internal/steam/artwork/  Steam library art in userdata/<id>/config/grid/
internal/steam/process/  Whether Steam is running, per OS
internal/emulator/       Platform to emulator resolution and the capability map
internal/emulator/esde/  ES-DE system definitions and emulator find rules, generated
internal/launcher/       Runs an emulator and measures the session
internal/reconcile/      The queue-to-Steam loop: stage, apply, report
internal/romm/           RomM REST client and the Socket.IO subscription
internal/config/         Settings file
tools/gen-esde/          Regenerates the ES-DE tables from a pinned release
docs/DESIGN.md           Design document
```

## License

AGPL-3.0, matching RomM. See [LICENSE](LICENSE).

The generated emulator tables under `internal/emulator/esde/` derive from [ES-DE](https://gitlab.com/es-de/emulationstation-de), MIT licensed; its notice is in [LICENSE.ES-DE](internal/emulator/esde/LICENSE.ES-DE).
