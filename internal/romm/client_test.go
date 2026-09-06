package romm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExchangeNormalisesCodeAndReturnsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/client-tokens/exchange" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request %s %s auth=%q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["code"] != "ABC123" {
			t.Errorf("code not normalised: %q", body["code"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "name": "Desktop", "raw_token": "tok", "scopes": []string{"devices.write"}})
	}))
	defer srv.Close()

	out, err := ExchangePairCode(context.Background(), srv.URL+"/", "abc-123")
	if err != nil || out.RawToken != "tok" {
		t.Fatalf("exchange: out=%+v err=%v", out, err)
	}
}

func TestDownloadFileIsAtomicAndCached(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/api/roms/7/files/content/Game%20%28USA%29.sfc" && r.URL.Path != "/api/roms/7/files/content/Game (USA).sfc" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		_, _ = w.Write([]byte("ROMDATA"))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "test")
	dir := t.TempDir()
	f := RomFile{ID: 7, FileName: "Game (USA).sfc"}
	p, err := c.DownloadFile(context.Background(), f, dir)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(dir, "Game (USA).sfc") {
		t.Fatalf("dest %q", p)
	}
	if b, _ := os.ReadFile(p); string(b) != "ROMDATA" {
		t.Fatalf("content %q", b)
	}
	if _, err := os.Stat(p + ".part"); err == nil {
		t.Fatal("temp file left behind")
	}
	if _, err := c.DownloadFile(context.Background(), f, dir); err != nil || hits != 1 {
		t.Fatalf("second download should hit cache: hits=%d err=%v", hits, err)
	}
}

func TestAPIErrorCarriesStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"nope"}`, http.StatusConflict)
	}))
	defer srv.Close()
	err := New(srv.URL, "t", "v").AckShortcut(context.Background(), 1, "added", nil, "")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != 409 {
		t.Fatalf("want APIError 409, got %v", err)
	}
}

func TestGetConfigReadsTheCoreMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"EJS_CORES":{"snes":["snes9x","bsnes"]},"EJS_NIGHTLY_CORES":{"3ds":["citra"]}}`))
	}))
	defer srv.Close()

	out, err := New(srv.URL, "tok", "test").GetConfig(context.Background())
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if got := out.EJSCores["snes"]; len(got) != 2 || got[0] != "snes9x" {
		t.Errorf("stable cores: %v", got)
	}
	if got := out.EJSNightlyCores["3ds"]; len(got) != 1 || got[0] != "citra" {
		t.Errorf("nightly cores: %v", got)
	}
}

func TestGetSteamArtworkAndImageDownload(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/shortcuts/artwork/7":
			_, _ = w.Write([]byte(`{"url_hero":"` + srv.URL + `/hero.png","url_logo":null}`))
		case "/hero.png":
			_, _ = w.Write([]byte("HERO"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok", "test")
	art, err := c.GetSteamArtwork(context.Background(), 7)
	if err != nil {
		t.Fatalf("get artwork: %v", err)
	}
	if art.URLLogo != "" {
		t.Errorf("a null slot should read as empty, got %q", art.URLLogo)
	}
	if got := c.DownloadImage(context.Background(), art.URLHero); string(got) != "HERO" {
		t.Errorf("hero download = %q", got)
	}
	if got := c.DownloadImage(context.Background(), art.URLLogo); got != nil {
		t.Errorf("an empty URL should download nothing, got %q", got)
	}
	if got := c.DownloadImage(context.Background(), srv.URL+"/missing.png"); got != nil {
		t.Errorf("a 404 should download nothing, got %q", got)
	}
}
