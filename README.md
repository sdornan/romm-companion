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

The server side (shortcut table, routes, socket event, capability column) is on a RomM branch and is not merged yet, so `run` has nothing to talk to against a stock RomM.

## How `run` works

`run` reports which platforms this PC can play, then waits for work. RomM emits `shortcuts:changed` over Socket.IO when the queue moves, and `run` also re-checks on a timer (`--interval`, 30s by default) so a dropped connection or a missed event costs latency rather than a lost change.

Each pass:

1. **Stage.** For every `pending_add` row: download the game's files and its cover, write the cover as Steam's vertical capsule, and report `staged`. A platform with no emulator on this PC fails that row alone, with the reason, and the rest of the pass continues.
2. **Apply.** If Steam is **not** running, open `shortcuts.vdf`, add the staged games and drop the removed ones, write it back atomically, then report `added` or `removed`. Shortcuts this tool did not create are never touched.
3. **Defer.** If Steam **is** running, nothing is written: Steam reads that file at startup and rewrites it on exit, so a write underneath it is lost. The change stays staged and lands on the next pass after Steam closes.

Reporting happens after the write, so an interrupted pass leaves rows queued rather than claiming a shortcut that is not there.

## Build

Go 1.24 or newer.

```sh
go build ./cmd/romm-companion
go test ./...
```

Cross-compile with `GOOS`/`GOARCH` as usual; there are no cgo dependencies.

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
internal/launcher/       Runs an emulator and measures the session
internal/reconcile/      The queue-to-Steam loop: stage, apply, report
internal/romm/           RomM REST client and the Socket.IO subscription
internal/config/         Settings file
docs/DESIGN.md           Design document
```

## License

AGPL-3.0, matching RomM. See [LICENSE](LICENSE).
