# RomM Companion

A small desktop agent that connects a gaming PC to a [RomM](https://github.com/rommapp/romm) server. It does two things:

- **Add to Steam.** Press a button in RomM and the game appears in your Steam library on this PC, with artwork, launching through a local emulator.
- **Play on Desktop.** Launch a RomM game on this PC without Steam, from a browser on the same machine or from your phone.

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
| `run` | Not yet. Needs the server-side shortcut queue described in the design. |

The server side (shortcut table, routes, socket events, capability column) is being built on a RomM branch and is not merged. Until it lands, the companion cannot receive "Add to Steam" requests; everything above still works.

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

4. `romm-companion launch --rom 123`

Settings live at `$ROMM_COMPANION_CONFIG` or `<OS config dir>/romm-companion/config.json`.

## Layout

```
cmd/romm-companion/      CLI entry point and commands
internal/steam/vdf/      Binary KeyValues reader and writer
internal/steam/shortcuts/ shortcuts.vdf entries, app id derivation, RomM ownership tags
internal/steam/paths/    Steam install and userdata discovery per OS
internal/emulator/       Platform to emulator resolution and the capability map
internal/launcher/       Runs an emulator and measures the session
internal/romm/           RomM REST client (pairing, devices, files, play sessions, shortcut queue)
internal/config/         Settings file
docs/DESIGN.md           Design document
```

## License

AGPL-3.0, matching RomM. See [LICENSE](LICENSE).
