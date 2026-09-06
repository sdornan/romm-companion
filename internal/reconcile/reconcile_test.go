package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sdornan/romm-companion/internal/config"
	"github.com/sdornan/romm-companion/internal/emulator"
	"github.com/sdornan/romm-companion/internal/romm"
	"github.com/sdornan/romm-companion/internal/steam/artwork"
	"github.com/sdornan/romm-companion/internal/steam/paths"
	"github.com/sdornan/romm-companion/internal/steam/shortcuts"
	"github.com/sdornan/romm-companion/internal/steam/vdf"
)

// server stands in for RomM: it serves one queue, one rom and its file, and
// records every ack so a test can assert what the device reported.
type server struct {
	mu    sync.Mutex
	queue []romm.Shortcut
	acks  []ack
	ts    *httptest.Server
}

type ack struct {
	ID     int
	Status string
	AppID  *uint32
	Error  string
}

func newServer(t *testing.T, queue []romm.Shortcut) *server {
	t.Helper()
	s := &server{queue: queue}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/shortcuts", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(s.queue)
	})
	mux.HandleFunc("/api/shortcuts/", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status     string  `json:"status"`
			SteamAppID *uint32 `json:"steam_app_id"`
			Error      string  `json:"error"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		var id int
		_, _ = fmtSscan(parts[len(parts)-2], &id)
		s.mu.Lock()
		s.acks = append(s.acks, ack{
			ID: id, Status: body.Status, AppID: body.SteamAppID, Error: body.Error,
		})
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	// Registered ahead of the ack catch-all: ServeMux prefers the longer pattern.
	mux.HandleFunc("/api/shortcuts/artwork/", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url_hero": s.ts.URL + "/sgdb/hero.png",
			"url_logo": s.ts.URL + "/sgdb/logo.png",
		})
	})
	mux.HandleFunc("/sgdb/hero.png", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("HERODATA"))
	})
	mux.HandleFunc("/sgdb/logo.png", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("LOGODATA"))
	})
	mux.HandleFunc("/api/roms/7", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(romm.Rom{
			ID:           7,
			Name:         "Chrono Trigger",
			PlatformSlug: "snes",
			FsName:       "ct.sfc",
			FsNameNoExt:  "ct",
			PathCoverL:   "roms/snes/7/cover-l.png",
			Files:        []romm.RomFile{{ID: 70, FileName: "ct.sfc"}},
		})
	})
	mux.HandleFunc("/api/roms/70/files/content/ct.sfc", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ROMDATA"))
	})
	mux.HandleFunc("/assets/romm/resources/roms/snes/7/cover-l.png",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("PNGDATA"))
		})

	s.ts = httptest.NewServer(mux)
	t.Cleanup(s.ts.Close)
	return s
}

// fmtSscan keeps the import list short; the ack path is ".../<id>/ack".
func fmtSscan(s string, out *int) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(r-'0')
	}
	*out = n
	return 1, nil
}

func (s *server) ackFor(id int) []ack {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ack
	for _, a := range s.acks {
		if a.ID == id {
			out = append(out, a)
		}
	}
	return out
}

func newEngine(t *testing.T, s *server, steamRunning bool) (*Engine, string) {
	t.Helper()
	root := t.TempDir()
	userDir := filepath.Join(root, "userdata", "1")
	if err := os.MkdirAll(filepath.Join(userDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &emulator.Resolver{
		Templates: map[string]string{"snes": `emu "%ROM%"`},
		Cores:     map[string][]string{},
	}
	return &Engine{
		Client:       romm.New(s.ts.URL, "tok", "test"),
		Cfg:          &config.Config{DownloadDir: filepath.Join(root, "roms")},
		Resolver:     resolver,
		ExePath:      filepath.Join(root, "romm-companion"),
		UserDataDir:  userDir,
		SteamRunning: func() (bool, error) { return steamRunning, nil },
		Logf:         t.Logf,
	}, userDir
}

func ownedShortcuts(t *testing.T, userDir string) []shortcuts.Shortcut {
	t.Helper()
	data, err := os.ReadFile(paths.ShortcutsFile(userDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	f, err := shortcuts.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return f.Owned()
}

func TestAddWritesShortcutAndReportsAppID(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, userDir := newEngine(t, s, false)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Staged != 1 || result.Added != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}

	owned := ownedShortcuts(t, userDir)
	if len(owned) != 1 {
		t.Fatalf("want one shortcut, got %d", len(owned))
	}
	if owned[0].AppName != "Chrono Trigger" || owned[0].RomID != 7 {
		t.Fatalf("shortcut = %+v", owned[0])
	}
	if owned[0].LaunchOptions != "launch --rom 7" {
		t.Fatalf("launch options = %q", owned[0].LaunchOptions)
	}

	// The rom and all three artwork slots are on disk.
	if _, err := os.Stat(filepath.Join(engine.Cfg.DownloadDir, "snes", "ct.sfc")); err != nil {
		t.Fatalf("rom not downloaded: %v", err)
	}
	for asset, want := range map[artwork.Asset]string{
		artwork.Capsule: "PNGDATA",
		artwork.Hero:    "HERODATA",
		artwork.Logo:    "LOGODATA",
	} {
		path := filepath.Join(paths.GridDir(userDir), artwork.FileName(owned[0].AppID, asset))
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s art not written: %v", asset, err)
		}
		if string(got) != want {
			t.Errorf("%s art = %q want %q", asset, got, want)
		}
	}

	acks := s.ackFor(1)
	if len(acks) != 2 || acks[0].Status != "staged" || acks[1].Status != "added" {
		t.Fatalf("acks = %+v", acks)
	}
	if acks[1].AppID == nil || *acks[1].AppID != owned[0].AppID {
		t.Fatalf("added ack should carry the app id, got %+v", acks[1])
	}
}

func TestStagesButDefersTheWriteWhileSteamRuns(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, userDir := newEngine(t, s, true)

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Staged != 1 || result.Added != 0 || !result.Pending() {
		t.Fatalf("result = %+v", result)
	}
	if got := ownedShortcuts(t, userDir); len(got) != 0 {
		t.Fatalf("nothing should be written while Steam runs, got %d", len(got))
	}
	acks := s.ackFor(1)
	if len(acks) != 1 || acks[0].Status != "staged" {
		t.Fatalf("acks = %+v", acks)
	}
}

func TestUnsupportedPlatformFailsTheRowNotThePass(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, _ := newEngine(t, s, false)
	engine.Resolver = &emulator.Resolver{Templates: map[string]string{}, Cores: map[string][]string{}}

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatalf("one bad row should not fail the pass: %v", err)
	}
	if result.Failed != 1 || result.Added != 0 {
		t.Fatalf("result = %+v", result)
	}
	acks := s.ackFor(1)
	if len(acks) != 1 || acks[0].Status != "failed" || acks[0].Error == "" {
		t.Fatalf("acks = %+v", acks)
	}
}

func TestRemoveDropsTheShortcutAndItsArt(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, userDir := newEngine(t, s, false)
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	appID := ownedShortcuts(t, userDir)[0].AppID

	s.mu.Lock()
	s.queue = []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_remove"}}
	s.mu.Unlock()

	result, err := engine.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got := ownedShortcuts(t, userDir); len(got) != 0 {
		t.Fatalf("shortcut should be gone, got %d", len(got))
	}
	art := filepath.Join(paths.GridDir(userDir), artwork.FileName(appID, artwork.Capsule))
	if _, err := os.Stat(art); !os.IsNotExist(err) {
		t.Fatal("artwork should be gone with the shortcut")
	}
	acks := s.ackFor(1)
	if acks[len(acks)-1].Status != "removed" {
		t.Fatalf("acks = %+v", acks)
	}
}

func TestForeignShortcutsSurviveAPass(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, userDir := newEngine(t, s, false)

	// A shortcut the user added by hand: no romm tag, so nothing this program
	// owns, and it must come through a pass untouched.
	root := &vdf.Node{Key: "shortcuts", Type: vdf.TypeObject}
	entry := root.SetObject("0")
	entry.SetInt("appid", -5)
	entry.SetString("AppName", "Handmade")
	entry.SetString("Exe", `"/bin/true"`)
	var buf bytes.Buffer
	if err := vdf.Encode(&buf, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ShortcutsFile(userDir), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(paths.ShortcutsFile(userDir))
	if err != nil {
		t.Fatal(err)
	}
	f, err := shortcuts.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if f.Len() != 2 {
		t.Fatalf("want the handmade entry plus ours, got %d", f.Len())
	}
	if owned := f.Owned(); len(owned) != 1 || owned[0].RomID != 7 {
		t.Fatalf("only the rom should be ours: %+v", owned)
	}
}

func TestDeleteOnRemoveClearsTheDownloadedFile(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, _ := newEngine(t, s, false)
	engine.Cfg.DeleteOnRemove = true
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(engine.Cfg.DownloadDir, "snes", "ct.sfc")
	if _, err := os.Stat(rom); err != nil {
		t.Fatalf("rom should be downloaded first: %v", err)
	}

	s.mu.Lock()
	s.queue = []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_remove"}}
	s.mu.Unlock()
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rom); !os.IsNotExist(err) {
		t.Fatal("rom file should be deleted when the setting is on")
	}
}

func TestDownloadIsKeptByDefault(t *testing.T) {
	s := newServer(t, []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_add"}})
	engine, _ := newEngine(t, s, false)
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.queue = []romm.Shortcut{{ID: 1, RomID: 7, Status: "pending_remove"}}
	s.mu.Unlock()
	if _, err := engine.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	rom := filepath.Join(engine.Cfg.DownloadDir, "snes", "ct.sfc")
	if _, err := os.Stat(rom); err != nil {
		t.Fatalf("rom should survive a removal by default: %v", err)
	}
}
