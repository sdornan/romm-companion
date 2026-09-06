# RomM Companion design

Revision 4, 6 September 2026. Two buttons on every game in RomM: **Add to Steam** and **Play on Desktop**. Both are served by one small companion app on the gaming PC that downloads the ROM, launches the right emulator, and reports playtime and saves back. Steam shortcuts are one way to reach that launcher; RomM's own Play menu is the other.

## Decision

One client, the desktop companion, framed as a RomM desktop agent rather than a Steam tool. It writes Steam's shortcut file itself rather than driving Steam ROM Manager, and it exposes the same launcher directly so games can start from RomM without Steam.

Ruled out:

- **Decky plugin.** Depends on the undocumented Steam client JavaScript API, which has broken on SteamOS updates.
- **Steam ROM Manager**, as a feed target, a runtime, or a data source. It is a batch re-parser bolted to an Electron GUI, requires Steam fully closed, wants one manifest file per title, and has no remote manifest support. Neither it nor EmuDeck is used as a data source.

### Write `shortcuts.vdf` directly (chosen)

- The binary VDF layout has not changed materially in a decade. The app id is a CRC32 of the quoted exe plus name with the high bit set; artwork is PNGs named by that id.
- Launch command, artwork and tags are set per shortcut, not per parser.
- Changes stage while Steam runs and apply on the next restart, so a user can add ten games and restart once.
- Gives up Steam ROM Manager's controller templates and categories. Both are extra config files Steam reads on restart and can be added later.

Steam only re-reads shortcuts on restart. The button therefore promises "Queued for Steam" until the companion confirms, never "In Steam" on click.

## What the player experiences

**First run.** Install the companion. It finds Steam and the active `userdata/<steamid>`, asks for a download folder, and resolves an emulator per platform. Enter the code from RomM's `/pair` page; the companion registers as a device and appears under Settings, Devices. It sits in the tray with a badge count of queued changes.

**Every day after.**

1. Open a game in RomM on any screen and press **Add to Steam**. If more than one PC is paired, pick which.
2. The companion downloads the ROM and artwork within seconds and shows "1 change ready. Restart Steam to apply." RomM shows "Queued for Steam".
3. Choose "Apply and restart Steam" from the tray, or quit Steam. The companion writes, Steam comes back with the game, cover, hero and logo. RomM flips to "In Steam".
4. Press Play in Steam. Saves pull from RomM, the emulator launches, on exit the play session and saves push back.
5. Or skip Steam: pick **Play on Desktop** in RomM's Play menu. On the same PC the browser hands off to the companion; from a phone the game starts on the desktop.
6. **Remove from Steam** queues the same way. Deleting the downloaded file is a per-device setting, off by default.

## The server contract

The web UI records intent; the companion reconciles and reports back. RomM already has device registration, token pairing, a socket for push events, per-file download, play session ingest and save sync. This adds one table, one column on devices, six routes and two events.

```
Web UI ── PUT /api/shortcuts ──> RomM API ── shortcuts:changed ──> Companion
                                    ^                                  │
                                    └──── POST /api/shortcuts/{id}/ack ┘
```

### Data model

```python
# backend/models/shortcut.py
class ShortcutStatus(enum.StrEnum):
    PENDING_ADD = "pending_add"       # user asked; companion has not applied
    STAGED = "staged"                 # files downloaded; waiting on Steam restart
    ADDED = "added"
    PENDING_REMOVE = "pending_remove"
    FAILED = "failed"

class LaunchMode(enum.StrEnum):
    EMULATOR = "emulator"      # companion resolves platform -> command
    WEB_PLAYER = "web_player"  # browser in app mode at RomM's EmulatorJS route

class Shortcut(BaseModel):
    __tablename__ = "shortcuts"
    __table_args__ = (UniqueConstraint("device_id", "rom_id"),)

    id: Mapped[int]
    user_id: Mapped[int]           # FK users, cascade
    device_id: Mapped[str]         # FK devices, cascade; the paired companion
    rom_id: Mapped[int]            # FK roms, cascade
    status: Mapped[ShortcutStatus]
    launch_mode: Mapped[LaunchMode | None]   # None = device default
    steam_app_id: Mapped[int | None]         # reported by the companion on ack
    error: Mapped[str | None]
    created_at / updated_at
```

Rows are scoped to a **device**, not a user: a desktop and a laptop want different emulators and download roots. `KNOWN_DEVICES` in `backend/models/device.py` gains a `steam-companion` entry. A JSON `launch_capabilities` column on `devices` holds the companion's per-platform emulator map.

### Endpoints

| Method and path | Caller | Scope | Behaviour |
| --- | --- | --- | --- |
| `GET /api/shortcuts?rom_id=` | Web UI | `roms.user.read` | Current user's rows for a game across devices. Drives button state. |
| `PUT /api/shortcuts` | Web UI | `roms.user.write` | Body `{device_id, rom_id, launch_mode?}`. Upserts to `pending_add`. Hidden ROMs are 404-masked. |
| `DELETE /api/shortcuts/{id}` | Web UI | `roms.user.write` | Sets `pending_remove`. Row deleted only after the companion acks. |
| `GET /api/shortcuts?device_id=me&status=pending_add,pending_remove,staged` | Companion | `devices.read` | Work queue. Polled on startup and after reconnect. |
| `PUT /api/devices/{id}` | Companion | `devices.write` | Existing route; gains `launch_capabilities` in the body. |
| `POST /api/devices/{id}/launch` | Web UI | `roms.user.write` | Body `{rom_id}`. Emits `device:launch`. 409 if offline or unsupported platform. |
| `POST /api/shortcuts/{id}/ack` | Companion | `devices.write` | Body `{status: staged \| added \| removed \| failed, steam_app_id?, error?}`. `removed` deletes the row. |
| `GET /api/config/emulator-cores` | Companion | none | RomM's platform-to-libretro-core map, so the companion never carries a stale copy. |

Socket events: `shortcuts:changed` carries `{device_id}` only, meaning "go fetch your queue". `device:launch` carries `{rom_id, launch_id}`; its siblings `device:save-state`, `device:save-and-exit` and `device:volume` mirror the streaming broker's verbs for remote control from another device.

### Button states in GameActions

| State | Row status | Notes |
| --- | --- | --- |
| Add to Steam | none | Hidden entirely when the user has no paired companion. |
| Queued for Steam | `pending_add` | Companion has not picked it up, or is downloading. |
| Restart Steam to apply | `staged` | Files on disk; waiting for Steam to close. |
| In Steam | `added` | Remove moves to the overflow menu with a confirmation. |
| Add to Steam (disabled) | none | Tooltip "No emulator set up for Nintendo Switch on Desktop" when `launch_capabilities` has no entry. |
| Steam add failed | `failed` | Tooltip shows `error`; click retries. |

With two or more paired devices the button opens a picker. Settings, Devices gets a card per companion: last seen, shortcuts synced, pending changes, default launch mode, "Remove all from Steam".

## Play on Desktop

Two ways in, both ending at `romm-companion launch --rom <id>`.

**Same PC: a `romm://` URL scheme.** The installer registers the scheme (registry key on Windows, `.desktop` entry with `x-scheme-handler/romm` on Linux, `CFBundleURLTypes` on macOS). RomM's Play menu shows **Play on Desktop** whenever the user has a paired companion; clicking opens `romm://play?rom=123&server=<origin>`. The browser asks once whether to open the companion. The companion checks the origin against the one it paired with, downloads if needed, launches. A browser cannot tell whether the handler exists, so after a couple of seconds RomM shows "Nothing opened? Install the companion or choose a device".

**Any device: a launch command.** From a phone, the Play menu lists paired companions that are online and report a capability for the platform. Choosing one calls `POST /api/devices/{id}/launch`; the server emits `device:launch`; the companion launches and posts the play session on exit. RomM shows "Playing on Desktop" while the session is open.

Launching directly gives you the emulator alone. Steam Input, the overlay, Steam's playtime and Remote Play only exist when Steam launches the game, so both paths stay.

## Emulator mapping

Three layers. Two are answered by data that already exists; the third is a scan of the local machine.

1. **Platform to RetroArch core.** RomM's EmulatorJS core map in `frontend/src/utils/index.ts` already keys libretro core names on platform slugs. Desktop RetroArch uses the same names, so the default command for every browser-playable platform is `retroarch -L <cores>/<core>_libretro.<ext> "<rom>"`. Exposed at `GET /api/config/emulator-cores`.
2. **Platform to standalone emulator.** ES-DE's `es_systems.xml` covers about 150 systems per OS with labelled alternatives and `%ROM%`, `%EMULATOR_RETROARCH%`, `%CORE_RETROARCH%` placeholders. MIT licensed. The companion ships a translated copy, refreshed each release, joined to RomM slugs by the mapping RomM's ES-DE gamelist exporter already uses.
3. **Emulator to a path on this PC.** ES-DE's `es_find_rules.xml` lists where each emulator lives per OS. The companion walks the rules on first run and on demand.

Resolution order per platform: the user's explicit template; a detected standalone, taking ES-DE's first-listed alternative found on disk; RetroArch with the core from RomM's map; the web player; none. Multi-disc games pass the `.m3u` or `.cue`, never a bare `.bin`.

The companion reports its resolved map as `launch_capabilities`, platform slug to a descriptor such as `retroarch:snes9x`, `standalone:pcsx2`, `web_player`, or null.

## The companion

Single small Go binary with a tray icon. Configuration on a big screen lives in RomM; the companion's own UI is pairing, folders, templates and "Apply and restart Steam".

- **Steam discovery.** Registry on Windows; `~/.steam/steam`, `~/.local/share/Steam` or the Flatpak path on Linux; `~/Library/Application Support/Steam` on macOS. Most recently used `userdata/<steamid>` wins, user can override.
- **VDF writer.** Parse the existing file, merge RomM-owned entries by a `romm:<rom_id>` tag, never touch shortcuts it did not create. Temp file and rename. Refuse to write while Steam is running.
- **Artwork.** RomM's cover as the vertical capsule; hero and logo from SteamGridDB via the stored `sgdb_id`, fetched through RomM so the key stays server-side. Saved as `<appid>p.png`, `<appid>_hero.png`, `<appid>_logo.png` in `grid/`.
- **Launcher.** One entry point reached three ways: the Steam shortcut exe, the `romm://` scheme, the `device:launch` event. Pulls saves via `/api/sync/negotiate`, runs the command, pushes saves, posts to `/api/play-sessions`.
- **Restart handling.** Watches Steam's process; writes when it exits with changes staged, relaunches if opted in. On Windows waits for the process handle to close, not the window.

## Relation to streaming and existing clients

| | Emulator streaming broker | Desktop companion | Playnite plugin |
| --- | --- | --- | --- |
| Where the emulator runs | Docker container next to RomM | The user's own PC | The user's PC, via Playnite |
| What the user sees | Selkies WebRTC stream in the tab | Emulator on that screen, or Steam | Playnite's library |
| Who sets it up | Admin, `config.yml` | Any user, by pairing | Any user, in Playnite |
| How RomM talks to it | Server-to-server HTTP | Socket events to a paired device | Plugin polls REST |
| Solves | Heavy platforms from any browser | Use the gaming PC and its Steam library | RomM inside Playnite |

Borrow from the broker its verbs (launch, save-state, save-and-exit, volume), not its transport; borrow the Play menu slot from `useGameActions`, which already prefers streaming over EmulatorJS; read the Playnite plugin's device and download code before writing the equivalent here.

## Open decisions

- **Editing templates from RomM.** Overrides live in the companion for v1; the device card could edit them later.
- **Categories and controller templates.** Extra files Steam reads on restart. Not v1.
- **Delete the ROM on remove?** Default off, per-device toggle.
- **Scope naming.** Reuse `roms.user.*` and `devices.*`, or add a `shortcuts.*` pair.
- **Scheme hardening.** Any web page can open a `romm://` link. The companion accepts only ROM ids from its paired server and prompts on a new origin. Whether a one-time nonce is also needed deserves a security pass before the companion ships.

## Phasing

1. **Server contract** (RomM backend). Model and migration, devices column, six routes, core-map endpoint, two socket events, tests, known device type.
2. **Web UI** (RomM frontend v2). GameActions states, Play on Desktop entry with the handler-missing fallback, device picker, Devices card, i18n, Storybook.
3. **Companion core** (this repo). Pairing, reconcile loop, VDF writer, artwork, emulator resolution, capability report, launcher, `romm://` scheme, socket handler, restart handling. Linux first.
4. **Windows and macOS.** Steam discovery per OS, installers, signing, autostart, tray polish.
